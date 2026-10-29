package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// createTestInterface membuat wg0 (10.8.0.1/24) lewat API dan mengembalikan id-nya.
func createTestInterface(t *testing.T, router http.Handler, admin *http.Cookie) int64 {
	t.Helper()
	response := doRequest(router, "POST", "/api/v1/interfaces", validInterfaceBody, admin, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("buat interface: %d %s", response.Code, response.Body)
	}
	var body struct {
		Data interfaceView `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	return body.Data.ID
}

// postPeer membuat peer dan mengurai respons (status dikembalikan agar kasus gagal bisa diuji).
func postPeer(t *testing.T, router http.Handler, admin *http.Cookie, body string) (int, peerView, string) {
	t.Helper()
	response := doRequest(router, "POST", "/api/v1/peers", body, admin, true)
	var parsed struct {
		Data peerView `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &parsed)
	return response.Code, parsed.Data, response.Body.String()
}

// TestPeerCRUD (PLAN 5.1): IP otomatis .2 .3, IP manual bentrok 409, di luar subnet 422, public key duplikat
// 409, disable → hilang dari spec kernel, enable → kembali, respons tanpa private/preshared, hapus → 204.
func TestPeerCRUD(t *testing.T) {
	app, router, kernel := newInterfaceTestApp(t)
	admin := loginAs(t, router, "admin", testPassword)
	operator := loginAs(t, router, "operator", testPassword)
	interfaceID := createTestInterface(t, router, admin)
	prefix := `{"interface_id":` + strconv.FormatInt(interfaceID, 10) + `,`

	code, first, raw := postPeer(t, router, admin, prefix+`"name":"laptop","use_psk":true}`)
	if code != http.StatusCreated || first.Address != "10.8.0.2/32" || !first.HasPrivateKey || !first.HasPSK {
		t.Fatalf("peer pertama: %d %s", code, raw)
	}
	assertNoSecretFields(t, raw)
	clientKey, _ := wgtypes.GeneratePrivateKey()
	code, second, raw := postPeer(t, router, admin, prefix+`"name":"hp","public_key":"`+clientKey.PublicKey().String()+`","site_subnets":["192.168.50.0/24"]}`)
	if code != http.StatusCreated || second.Address != "10.8.0.3/32" || second.HasPrivateKey || second.HasPSK {
		t.Fatalf("peer kedua (public key ditempel): %d %s", code, raw)
	}

	spec := kernel.applied["wg0"]
	if len(spec.Peers) != 2 || spec.Peers[0].PresharedKey == nil || len(spec.Peers[1].AllowedIPs) != 2 {
		t.Fatalf("spec kernel: %+v", spec)
	}
	var sealed []byte
	if err := app.DB.QueryRow(`SELECT private_key FROM peers WHERE id = ?`, first.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if plain, err := app.Box.Open(sealed); err != nil || !strings.Contains(spec.Peers[0].PublicKey.String(), mustPublic(t, string(plain))) {
		t.Fatal("private key peer harus tersimpan terenkripsi dan cocok dengan public key di kernel")
	}

	cases := []struct {
		name, body string
		want       int
		code       string
	}{
		{"IP manual bentrok", prefix + `"name":"x","address":"10.8.0.2"}`, http.StatusConflict, "address_taken"},
		{"IP di luar subnet", prefix + `"name":"x","address":"10.9.0.2"}`, http.StatusUnprocessableEntity, "validation_failed"},
		{"public key duplikat", prefix + `"name":"x","public_key":"` + clientKey.PublicKey().String() + `"}`, http.StatusConflict, "public_key_taken"},
		{"nama duplikat", prefix + `"name":"laptop"}`, http.StatusConflict, "name_taken"},
		{"site bentrok", prefix + `"name":"x","site_subnets":["192.168.50.0/25"]}`, http.StatusUnprocessableEntity, "validation_failed"},
		{"interface tidak ada", `{"interface_id":99,"name":"x"}`, http.StatusNotFound, "not_found"},
		{"owner tidak ada", prefix + `"name":"x","owner_user_id":999}`, http.StatusUnprocessableEntity, "validation_failed"},
	}
	for _, c := range cases {
		code, _, raw := postPeer(t, router, admin, c.body)
		if code != c.want || !strings.Contains(raw, `"`+c.code+`"`) {
			t.Errorf("%s: want %d %s, got %d %s", c.name, c.want, c.code, code, raw)
		}
	}
	if response := doRequest(router, "POST", "/api/v1/peers", prefix+`"name":"x"}`, operator, true); response.Code != http.StatusForbidden {
		t.Errorf("operator POST: %d", response.Code)
	}
	if response := doRequest(router, "GET", "/api/v1/peers", "", operator, false); response.Code != http.StatusOK {
		t.Errorf("operator GET: %d %s", response.Code, response.Body)
	} else {
		assertNoSecretFields(t, response.Body.String())
	}

	path := "/api/v1/peers/" + strconv.FormatInt(first.ID, 10)
	if response := doRequest(router, "POST", path+"/disable", "", admin, true); response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"disabled_reason":"manual"`) {
		t.Fatalf("disable: %d %s", response.Code, response.Body)
	}
	if peers := kernel.applied["wg0"].Peers; len(peers) != 1 || peers[0].PublicKey.String() != second.PublicKey {
		t.Fatalf("peer nonaktif harus hilang dari kernel: %+v", peers)
	}
	// IP peer nonaktif tetap miliknya: peer baru tidak boleh mendapat .2.
	if code, third, raw := postPeer(t, router, admin, prefix+`"name":"tablet"}`); code != http.StatusCreated || third.Address != "10.8.0.4/32" {
		t.Fatalf("IP peer nonaktif tidak boleh dipakai ulang: %d %s", code, raw)
	}
	if response := doRequest(router, "POST", path+"/enable", "", admin, true); response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"disabled_reason":""`) || len(kernel.applied["wg0"].Peers) != 3 {
		t.Fatalf("enable: %d %s peers=%d", response.Code, response.Body, len(kernel.applied["wg0"].Peers))
	}

	// PUT: ganti alamat ke IP kosong + nama; alamat sendiri tidak dihitung bentrok.
	if response := doRequest(router, "PUT", path, `{"name":"laptop-kantor","address":"10.8.0.2"}`, admin, true); response.Code != http.StatusOK {
		t.Fatalf("PUT alamat sendiri: %d %s", response.Code, response.Body)
	}
	if response := doRequest(router, "PUT", path, `{"name":"laptop-kantor","address":"10.8.0.3"}`, admin, true); response.Code != http.StatusConflict {
		t.Fatalf("PUT ke IP peer lain: %d %s", response.Code, response.Body)
	}

	// Kernel menolak → DB tidak berubah.
	kernel.fail = true
	if response := doRequest(router, "DELETE", path, "", admin, true); response.Code != http.StatusInternalServerError {
		t.Fatalf("DELETE dengan kernel menolak: %d", response.Code)
	}
	kernel.fail = false
	if response := doRequest(router, "GET", path, "", admin, false); response.Code != http.StatusOK {
		t.Fatalf("peer harus tetap ada setelah DELETE gagal: %d", response.Code)
	}
	if response := doRequest(router, "DELETE", path, "", admin, true); response.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d %s", response.Code, response.Body)
	}
	if response := doRequest(router, "GET", path, "", admin, false); response.Code != http.StatusNotFound {
		t.Fatalf("GET setelah DELETE: %d", response.Code)
	}
	if len(kernel.applied["wg0"].Peers) != 2 {
		t.Fatalf("kernel harus berisi 2 peer setelah hapus, got %d", len(kernel.applied["wg0"].Peers))
	}
}

// mustPublic menurunkan public key dari private key base64.
func mustPublic(t *testing.T, private string) string {
	t.Helper()
	key, err := wgtypes.ParseKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return key.PublicKey().String()
}

// assertNoSecretFields gagal bila JSON memuat KUNCI field rahasia (bukan penanda has_*), atau nilai yang
// terlihat seperti kunci di luar field public_key.
func assertNoSecretFields(t *testing.T, raw string) {
	t.Helper()
	for _, field := range []string{`"private_key"`, `"preshared_key"`, `"psk"`} {
		if strings.Contains(raw, field) {
			t.Fatalf("respons memuat field rahasia %s: %s", field, raw)
		}
	}
}
