package api

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/peer"
	"wgpanel/internal/wgconf"
)

// TestPeerConfig (PLAN 5.2): isi .conf = RenderClient(BuildClientConfig) byte-exact dan memakai kunci yang sama
// dengan kernel, QR berupa PNG, header no-store + lampiran bernama aman, operator 403, public key ditempel →
// placeholder private key.
func TestPeerConfig(t *testing.T) {
	app, router, kernel := newInterfaceTestApp(t)
	admin := loginAs(t, router, "admin", testPassword)
	operator := loginAs(t, router, "operator", testPassword)
	interfaceID := createTestInterface(t, router, admin)
	prefix := `{"interface_id":` + strconv.FormatInt(interfaceID, 10) + `,`
	_, created, raw := postPeer(t, router, admin, prefix+`"name":"Laptop \"Bu Ani\"/2026","use_psk":true}`)
	if created.ID == 0 {
		t.Fatalf("buat peer: %s", raw)
	}
	path := "/api/v1/peers/" + strconv.FormatInt(created.ID, 10)

	response := doRequest(router, "GET", path+"/config", "", admin, false)
	if response.Code != http.StatusOK {
		t.Fatalf("config: %d %s", response.Code, response.Body)
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="Laptop--Bu-Ani--2026.conf"` {
		t.Fatalf("Content-Disposition: %q", got)
	}
	if response.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("header: %v", response.Header())
	}
	row, _ := app.Queries.GetPeer(t.Context(), created.ID)
	iface, _ := app.Queries.GetInterface(t.Context(), interfaceID)
	expected, err := peer.BuildClientConfig(iface, row, app.Box)
	if err != nil {
		t.Fatal(err)
	}
	if response.Body.String() != wgconf.RenderClient(expected) {
		t.Fatalf("isi .conf tidak byte-exact:\n%s", response.Body)
	}
	// Kunci di .conf harus pasangan kunci yang diterapkan ke kernel (server = public key interface, PSK sama).
	spec := kernel.applied["wg0"]
	clientPrivate, _ := wgtypes.ParseKey(expected.PrivateKey)
	if clientPrivate.PublicKey() != spec.Peers[0].PublicKey || expected.PresharedKey != spec.Peers[0].PresharedKey.String() ||
		expected.ServerPublicKey != spec.PrivateKey.PublicKey().String() {
		t.Fatal("kunci di .conf tidak cocok dengan spec kernel")
	}
	for _, want := range []string{"Endpoint = vpn.example.id:51820", "AllowedIPs = 0.0.0.0/0", "DNS = 1.1.1.1", "PersistentKeepalive = 25"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf(".conf tidak memuat %q", want)
		}
	}

	qr := doRequest(router, "GET", path+"/qr", "", admin, false)
	if qr.Code != http.StatusOK || qr.Header().Get("Content-Type") != "image/png" || qr.Header().Get("Cache-Control") != "no-store" ||
		!bytes.HasPrefix(qr.Body.Bytes(), []byte("\x89PNG")) {
		t.Fatalf("qr: %d %v", qr.Code, qr.Header())
	}
	for _, suffix := range []string{"/config", "/qr"} {
		if response := doRequest(router, "GET", path+suffix, "", operator, false); response.Code != http.StatusForbidden {
			t.Errorf("operator %s: %d", suffix, response.Code)
		}
	}

	clientKey, _ := wgtypes.GeneratePrivateKey()
	_, pasted, _ := postPeer(t, router, admin, prefix+`"name":"hp","public_key":"`+clientKey.PublicKey().String()+`","client_allowed_ips":["10.8.0.0/24"]}`)
	body := doRequest(router, "GET", "/api/v1/peers/"+strconv.FormatInt(pasted.ID, 10)+"/config", "", admin, false).Body.String()
	if !strings.Contains(body, "PrivateKey = "+peer.PrivateKeyPlaceholder) || strings.Contains(body, "PresharedKey") ||
		!strings.Contains(body, "AllowedIPs = 10.8.0.0/24") {
		t.Fatalf("public key ditempel: %s", body)
	}
	if response := doRequest(router, "GET", "/api/v1/peers/99/config", "", admin, false); response.Code != http.StatusNotFound {
		t.Errorf("peer tidak ada: %d", response.Code)
	}
}

// TestSafeFileName: hanya [A-Za-z0-9._-], maks 40, tidak kosong, tanpa titik di ujung.
func TestSafeFileName(t *testing.T) {
	for input, want := range map[string]string{
		"laptop": "laptop", "Bu Ani\r\n": "Bu-Ani--", `a"b`: "a-b", "..": "peer", "": "peer",
		strings.Repeat("x", 50): strings.Repeat("x", 40), "héllo": "h-llo",
	} {
		if got := peer.SafeFileName(input); got != want {
			t.Errorf("%q: want %q got %q", input, want, got)
		}
	}
}
