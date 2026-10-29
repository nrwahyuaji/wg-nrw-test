package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// TestTraffic (PLAN 8.2): data sintetis 48 jam (1 byte rx + 2 byte tx per jam). range=24h → tepat 24 titik per jam,
// 7d → 7 titik di batas hari WITA, jam tanpa data = 0; range salah → 422; dashboard menjumlah hari ini + top 5.
func TestTraffic(t *testing.T) {
	app, router, _ := newInterfaceTestApp(t)
	admin := loginAs(t, router, "admin", testPassword)
	interfaceID := createTestInterface(t, router, admin)
	prefix := `{"interface_id":` + strconv.FormatInt(interfaceID, 10) + `,`
	_, peerA, raw := postPeer(t, router, admin, prefix+`"name":"a"}`)
	if peerA.ID == 0 {
		t.Fatalf("buat peer: %s", raw)
	}
	_, peerB, _ := postPeer(t, router, admin, prefix+`"name":"b"}`)

	// 2026-10-09 10.30 WITA = 02.30 UTC. Awal hari WITA = 2026-10-08 16.00 UTC.
	fixed := time.Date(2026, 10, 9, 2, 30, 0, 0, time.UTC)
	original := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = original })
	currentHour := fixed.Unix() - fixed.Unix()%3600
	for index := range int64(48) {
		if _, err := app.DB.Exec(`INSERT INTO traffic_hourly (peer_id, hour, rx_bytes, tx_bytes) VALUES (?, ?, 1, 2)`,
			peerA.ID, currentHour-index*3600); err != nil {
			t.Fatal(err)
		}
	}
	// Peer b: satu jam besar hari ini → harus jadi top 1; jam berjalan kosong untuk peer a dihapus agar ada "bolong".
	if _, err := app.DB.Exec(`INSERT INTO traffic_hourly (peer_id, hour, rx_bytes, tx_bytes) VALUES (?, ?, 1000, 0)`,
		peerB.ID, currentHour); err != nil {
		t.Fatal(err)
	}
	if _, err := app.DB.Exec(`DELETE FROM traffic_hourly WHERE peer_id = ? AND hour = ?`, peerA.ID, currentHour-3*3600); err != nil {
		t.Fatal(err)
	}

	get := func(path string) (int, []trafficPoint) {
		response := doRequest(router, "GET", path, "", admin, false)
		var body struct {
			Data []trafficPoint `json:"data"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		return response.Code, body.Data
	}
	peerPath := "/api/v1/peers/" + strconv.FormatInt(peerA.ID, 10) + "/traffic"

	code, hourly := get(peerPath + "?range=24h")
	if code != http.StatusOK || len(hourly) != 24 {
		t.Fatalf("24h: %d, %d titik", code, len(hourly))
	}
	if hourly[23].At != currentHour || hourly[0].At != currentHour-23*3600 {
		t.Errorf("24h: titik pertama/terakhir salah: %d..%d", hourly[0].At, hourly[23].At)
	}
	if hourly[20].RxBytes != 0 || hourly[20].TxBytes != 0 || hourly[19].RxBytes != 1 || hourly[23].TxBytes != 2 {
		t.Errorf("24h: jam kosong harus 0, jam lain 1/2: %+v %+v %+v", hourly[19], hourly[20], hourly[23])
	}

	code, daily := get(peerPath + "?range=7d")
	if code != http.StatusOK || len(daily) != 7 {
		t.Fatalf("7d: %d, %d titik", code, len(daily))
	}
	todayWITA := time.Date(2026, 10, 9, 0, 0, 0, 0, time.FixedZone("WITA", 8*3600)).Unix()
	if daily[6].At != todayWITA || daily[0].At != todayWITA-6*86400 {
		t.Errorf("7d: batas hari WITA salah: %d..%d, mau ..%d", daily[0].At, daily[6].At, todayWITA)
	}
	// 48 jam mundur dari 02.00 UTC (10.00 WITA, jam ke-11 hari ini) = 11 jam hari ini (1 dihapus) + 24 kemarin + 13 lusa.
	if daily[6].RxBytes != 10 || daily[5].RxBytes != 24 || daily[4].RxBytes != 13 || daily[3].RxBytes != 0 || daily[6].TxBytes != 20 {
		t.Errorf("7d: jumlah per hari WITA salah: %+v", daily)
	}
	if code, points := get(peerPath + "?range=30d"); code != http.StatusOK || len(points) != 30 {
		t.Errorf("30d: %d, %d titik", code, len(points))
	}

	// Interface = jumlah semua peer: jam berjalan = a(1) + b(1000).
	code, ifaceHourly := get("/api/v1/interfaces/" + strconv.FormatInt(interfaceID, 10) + "/traffic?range=24h")
	if code != http.StatusOK || len(ifaceHourly) != 24 || ifaceHourly[23].RxBytes != 1001 {
		t.Errorf("interface 24h: %d %+v", code, ifaceHourly)
	}
	if code, _ := get(peerPath + "?range=1y"); code != http.StatusUnprocessableEntity {
		t.Errorf("range tidak dikenal → 422, dapat %d", code)
	}

	response := doRequest(router, "GET", "/api/v1/dashboard", "", admin, false)
	var dashboard struct {
		Data dashboardView `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &dashboard)
	got := dashboard.Data
	if got.TodayRxBytes != 1010 || got.TodayTxBytes != 20 || len(got.TopPeers) != 2 ||
		got.TopPeers[0].ID != peerB.ID || got.TopPeers[1].RxBytes != 10 {
		t.Fatalf("dashboard trafik hari ini: %+v", got)
	}
}
