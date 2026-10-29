package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// TestStatusAndDashboard (PLAN 5.3): handshake 2 menit lalu → online, 4 menit → offline, peer tidak ada di kernel
// (nonaktif) → offline tanpa galat; dashboard menghitung total/aktif/online dan peer yang segera kedaluwarsa.
func TestStatusAndDashboard(t *testing.T) {
	app, router, kernel := newInterfaceTestApp(t)
	admin := loginAs(t, router, "admin", testPassword)
	interfaceID := createTestInterface(t, router, admin)
	fixed := time.Unix(1_800_000_000, 0)
	original := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = original })

	prefix := `{"interface_id":` + strconv.FormatInt(interfaceID, 10) + `,`
	ids := map[string]peerView{}
	for _, name := range []string{"baru", "lama", "mati", "belum"} {
		code, view, raw := postPeer(t, router, admin, prefix+`"name":"`+name+`"}`)
		if code != http.StatusCreated {
			t.Fatalf("buat %s: %s", name, raw)
		}
		ids[name] = view
	}
	kernel.handshakes = map[string]time.Time{
		ids["baru"].PublicKey: fixed.Add(-2 * time.Minute),
		ids["lama"].PublicKey: fixed.Add(-4 * time.Minute),
		ids["mati"].PublicKey: fixed.Add(-time.Minute),
	}
	if response := doRequest(router, "POST", "/api/v1/peers/"+strconv.FormatInt(ids["mati"].ID, 10)+"/disable", "", admin, true); response.Code != http.StatusOK {
		t.Fatalf("disable: %d", response.Code)
	}
	if _, err := app.DB.Exec(`UPDATE peers SET expires_at = ? WHERE id = ?`, fixed.Add(3*24*time.Hour).Unix(), ids["belum"].ID); err != nil {
		t.Fatal(err)
	}

	response := doRequest(router, "GET", "/api/v1/status", "", admin, false)
	if response.Code != http.StatusOK {
		t.Fatalf("status: %d %s", response.Code, response.Body)
	}
	var status struct {
		Data []peerStatusView `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &status)
	byID := map[int64]peerStatusView{}
	for _, row := range status.Data {
		byID[row.PeerID] = row
	}
	if len(status.Data) != 4 {
		t.Fatalf("status harus 4 baris: %+v", status.Data)
	}
	if !byID[ids["baru"].ID].Online || byID[ids["baru"].ID].LastHandshake == nil || byID[ids["baru"].ID].ReceiveBytes != 100 {
		t.Errorf("handshake 2 menit → online: %+v", byID[ids["baru"].ID])
	}
	if byID[ids["lama"].ID].Online {
		t.Errorf("handshake 4 menit → offline: %+v", byID[ids["lama"].ID])
	}
	if row := byID[ids["mati"].ID]; row.Online || row.LastHandshake != nil {
		t.Errorf("peer nonaktif (tidak di kernel) → offline tanpa data: %+v", row)
	}
	if row := byID[ids["belum"].ID]; row.Online || row.LastHandshake != nil {
		t.Errorf("belum pernah handshake → offline, last_handshake null: %+v", row)
	}

	response = doRequest(router, "GET", "/api/v1/dashboard", "", admin, false)
	var dashboard struct {
		Data dashboardView `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &dashboard)
	got := dashboard.Data
	if response.Code != http.StatusOK || got.PeersTotal != 4 || got.PeersEnabled != 3 || got.PeersOnline != 1 ||
		len(got.Interfaces) != 1 || got.Interfaces[0].Peers != 4 || got.Interfaces[0].PeersOnline != 1 || !got.Interfaces[0].Up {
		t.Fatalf("dashboard: %d %+v", response.Code, got)
	}
	if len(got.ExpiringSoon) != 1 || got.ExpiringSoon[0].ID != ids["belum"].ID {
		t.Fatalf("expiring_soon: %+v", got.ExpiringSoon)
	}

	// Interface hilang dari kernel: status tetap 200, semua offline.
	delete(kernel.applied, "wg0")
	response = doRequest(router, "GET", "/api/v1/status", "", admin, false)
	_ = json.Unmarshal(response.Body.Bytes(), &status)
	for _, row := range status.Data {
		if row.Online {
			t.Errorf("interface tidak di kernel → offline: %+v", row)
		}
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status tanpa kernel: %d", response.Code)
	}
}
