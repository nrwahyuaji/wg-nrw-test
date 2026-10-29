package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/sqlc"
)

// TestInterfaceHealth: sehat tanpa peringatan; peer full-tunnel + ip_forward 0 → ip_forward_off; interface
// hilang dari kernel → not_in_kernel; peer DB ≠ kernel → peer_mismatch.
func TestInterfaceHealth(t *testing.T) {
	app, router, kernel := newInterfaceTestApp(t)
	admin := loginAs(t, router, "admin", testPassword)
	forward := filepath.Join(t.TempDir(), "ip_forward")
	original := ipForwardPath
	ipForwardPath = forward
	t.Cleanup(func() { ipForwardPath = original })
	if err := os.WriteFile(forward, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	created := doRequest(router, "POST", "/api/v1/interfaces", validInterfaceBody, admin, true)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST: %d %s", created.Code, created.Body)
	}
	path := "/api/v1/interfaces/1/health"
	health := readHealth(t, router, path, admin)
	if !health.InKernel || !health.Up || health.ListenPortKernel != 51820 || len(health.Warnings) != 0 ||
		health.IPForward == nil || !*health.IPForward {
		t.Fatalf("sehat: %+v", health)
	}

	// Peer full-tunnel (ikut interface 0.0.0.0/0) + ip_forward 0 → peringatan. Peer ditambah langsung ke DB
	// sehingga kernel palsu belum tahu → juga peer_mismatch.
	key, _ := wgtypes.GeneratePrivateKey()
	if _, err := app.DB.Exec(`INSERT INTO peers (interface_id, name, address, public_key, created_at, updated_at)
		VALUES (1, 'a', '10.8.0.2/32', ?, 1, 1)`, key.PublicKey().String()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(forward, []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	health = readHealth(t, router, path, admin)
	if !slices.Contains(health.Warnings, warnIPForwardOff) || !slices.Contains(health.Warnings, warnPeerMismatch) ||
		health.PeersDB != 1 || health.PeersKernel != 0 {
		t.Fatalf("full-tunnel + forward 0: %+v", health)
	}

	delete(kernel.applied, "wg0")
	health = readHealth(t, router, path, admin)
	if health.InKernel || !slices.Contains(health.Warnings, warnNotInKernel) {
		t.Fatalf("hilang dari kernel: %+v", health)
	}

	// ip_forward tak terbaca → null, bukan galat.
	ipForwardPath = filepath.Join(t.TempDir(), "tidak-ada")
	if health = readHealth(t, router, path, admin); health.IPForward != nil {
		t.Fatalf("ip_forward harus null: %+v", health)
	}
	if response := doRequest(router, "GET", "/api/v1/interfaces/99/health", "", admin, false); response.Code != http.StatusNotFound {
		t.Fatalf("id tidak ada: %d", response.Code)
	}
}

// readHealth memanggil endpoint health dan mengurai data-nya.
func readHealth(t *testing.T, router http.Handler, path string, cookie *http.Cookie) healthView {
	t.Helper()
	response := doRequest(router, "GET", path, "", cookie, false)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, response.Code, response.Body)
	}
	var body struct {
		Data healthView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}

// TestHasFullTunnelPeer: kolom peer kosong ikut interface; peer split-tunnel tidak memicu peringatan.
func TestHasFullTunnelPeer(t *testing.T) {
	cases := []struct {
		iface, peer string
		want        bool
	}{
		{"0.0.0.0/0", "", true},
		{"10.8.0.0/24", "", false},
		{"0.0.0.0/0", "10.8.0.0/24", false},
		{"10.8.0.0/24", "0.0.0.0/0,10.0.0.0/8", true},
	}
	for index, c := range cases {
		if got := hasFullTunnelPeer(c.iface, []sqlc.Peer{{ClientAllowedIps: c.peer}}); got != c.want {
			t.Errorf("kasus %s: want %v got %v", strconv.Itoa(index), c.want, got)
		}
	}
}
