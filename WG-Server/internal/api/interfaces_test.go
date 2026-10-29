package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/reconcile"
	"wgpanel/internal/wg"
)

// recordingKernel adalah kernel palsu untuk uji handler: mencatat spec terakhir per nama dan bisa diminta gagal.
// handshakes (opsional) menyuntikkan waktu handshake per public key untuk uji status.
type recordingKernel struct {
	applied    map[string]wg.InterfaceSpec
	fail       bool
	handshakes map[string]time.Time
}

func (kernel *recordingKernel) Apply(spec wg.InterfaceSpec) error {
	if kernel.fail {
		return errors.New("kernel menolak")
	}
	kernel.applied[spec.Name] = spec
	return nil
}

func (kernel *recordingKernel) Remove(name string) error {
	delete(kernel.applied, name)
	return nil
}

// Inspect meniru kernel: interface ada bila pernah di-Apply dan belum di-Remove; peer = spec terakhir.
func (kernel *recordingKernel) Inspect(name string) (wg.InterfaceState, error) {
	spec, ok := kernel.applied[name]
	if !ok {
		return wg.InterfaceState{}, nil
	}
	peers := make([]wg.PeerStatus, 0, len(spec.Peers))
	for _, peer := range spec.Peers {
		key := peer.PublicKey.String()
		peers = append(peers, wg.PeerStatus{PublicKey: key, LastHandshake: kernel.handshakes[key], ReceiveBytes: 100, TransmitBytes: 200})
	}
	return wg.InterfaceState{Exists: true, Up: true, ListenPort: spec.ListenPort, Peers: peers}, nil
}

// newInterfaceTestApp = newTestApp + Reconciler dengan kernel palsu (uji handler tidak butuh root).
func newInterfaceTestApp(t *testing.T) (*App, http.Handler, *recordingKernel) {
	t.Helper()
	app, _ := newTestApp(t)
	kernel := &recordingKernel{applied: map[string]wg.InterfaceSpec{}}
	app.Reconciler = reconcile.New(app.DB, kernel, app.Box, "")
	app.Kernel = kernel
	return app, NewRouter(app), kernel
}

// validInterfaceBody adalah body POST yang lolos validasi; uji mengubah satu field per kasus.
const validInterfaceBody = `{"name":"wg0","address":"10.8.0.1/24","listen_port":51820,"endpoint_host":"vpn.example.id",` +
	`"client_dns":["1.1.1.1"],"client_allowed_ips":["0.0.0.0/0"]}`

// TestInterfaceCRUD (PLAN 4.2): 201, nama/port duplikat 409, CIDR salah 422, ganti subnet saat ada peer 422,
// operator POST 403, respons tanpa private key, dan kernel menolak → DB tidak berubah.
func TestInterfaceCRUD(t *testing.T) {
	app, router, kernel := newInterfaceTestApp(t)
	admin := loginAs(t, router, "admin", testPassword)
	operator := loginAs(t, router, "operator", testPassword)

	created := doRequest(router, "POST", "/api/v1/interfaces", validInterfaceBody, admin, true)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST: %d %s", created.Code, created.Body)
	}
	if strings.Contains(strings.ToLower(created.Body.String()), "private") {
		t.Fatalf("respons tidak boleh memuat private key: %s", created.Body)
	}
	var view struct {
		Data interfaceView `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Data.PublicKey == "" || view.Data.ClientKeepalive != 25 || !view.Data.Enabled {
		t.Fatalf("bawaan salah: %+v", view.Data)
	}
	if spec, ok := kernel.applied["wg0"]; !ok || spec.ListenPort != 51820 {
		t.Fatalf("kernel harus menerima wg0: %+v", kernel.applied)
	}
	var stored []byte
	if err := app.DB.QueryRow(`SELECT private_key FROM interfaces WHERE name = 'wg0'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if plain, err := app.Box.Open(stored); err != nil || string(plain) != kernel.applied["wg0"].PrivateKey.String() {
		t.Fatal("private key di DB harus terenkripsi dan sama dengan yang diterapkan ke kernel")
	}

	cases := []struct {
		name, body string
		cookie     *http.Cookie
		want       int
		code       string
	}{
		{"nama duplikat", strings.Replace(validInterfaceBody, `51820`, `51821`, 1), admin, http.StatusConflict, "name_taken"},
		{"port duplikat", strings.Replace(validInterfaceBody, `"wg0"`, `"wg1"`, 1), admin, http.StatusConflict, "port_taken"},
		{"CIDR salah", strings.Replace(validInterfaceBody, `10.8.0.1/24`, `10.8.0.0/24`, 1), admin, http.StatusUnprocessableEntity, "validation_failed"},
		{"nama salah", strings.Replace(validInterfaceBody, `"wg0"`, `"WG 0"`, 1), admin, http.StatusUnprocessableEntity, "validation_failed"},
		{"endpoint berskema", strings.Replace(validInterfaceBody, `vpn.example.id`, `https://vpn.example.id`, 1), admin, http.StatusUnprocessableEntity, "validation_failed"},
		{"MTU di luar batas", strings.Replace(validInterfaceBody, `"listen_port":51820`, `"listen_port":51825,"mtu":500`, 1), admin, http.StatusUnprocessableEntity, "validation_failed"},
		{"operator POST", strings.Replace(validInterfaceBody, `"wg0"`, `"wg9"`, 1), operator, http.StatusForbidden, "forbidden"},
	}
	for _, c := range cases {
		response := doRequest(router, "POST", "/api/v1/interfaces", c.body, c.cookie, true)
		if response.Code != c.want || !strings.Contains(response.Body.String(), `"`+c.code+`"`) {
			t.Errorf("%s: want %d %s, got %d %s", c.name, c.want, c.code, response.Code, response.Body)
		}
	}

	// Operator boleh membaca, tanpa private key.
	list := doRequest(router, "GET", "/api/v1/interfaces", "", operator, false)
	if list.Code != http.StatusOK || strings.Contains(strings.ToLower(list.Body.String()), "private") {
		t.Fatalf("GET operator: %d %s", list.Code, list.Body)
	}

	path := "/api/v1/interfaces/" + strconv.FormatInt(view.Data.ID, 10)
	// Ganti subnet tanpa peer → boleh; dengan peer → 422 subnet_locked.
	moved := strings.Replace(validInterfaceBody, `10.8.0.1/24`, `10.9.0.1/24`, 1)
	if response := doRequest(router, "PUT", path, moved, admin, true); response.Code != http.StatusOK {
		t.Fatalf("PUT ganti subnet tanpa peer: %d %s", response.Code, response.Body)
	}
	peerKey, _ := wgtypes.GeneratePrivateKey()
	if _, err := app.DB.Exec(`INSERT INTO peers (interface_id, name, address, public_key, created_at, updated_at)
		VALUES (?, 'a', '10.9.0.2/32', ?, 1, 1)`, view.Data.ID, peerKey.PublicKey().String()); err != nil {
		t.Fatal(err)
	}
	if response := doRequest(router, "PUT", path, validInterfaceBody, admin, true); response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), "subnet_locked") {
		t.Fatalf("PUT ganti subnet dengan peer: %d %s", response.Code, response.Body)
	}
	renamed := strings.Replace(moved, `"wg0"`, `"wg5"`, 1)
	if response := doRequest(router, "PUT", path, renamed, admin, true); response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), "name_immutable") {
		t.Fatalf("PUT ganti nama: %d %s", response.Code, response.Body)
	}

	// Kernel menolak → 500, DB tetap pada port lama.
	kernel.fail = true
	newPort := strings.Replace(moved, `51820`, `51830`, 1)
	if response := doRequest(router, "PUT", path, newPort, admin, true); response.Code != http.StatusInternalServerError {
		t.Fatalf("PUT dengan kernel menolak: %d %s", response.Code, response.Body)
	}
	var port int
	if err := app.DB.QueryRow(`SELECT listen_port FROM interfaces WHERE id = ?`, view.Data.ID).Scan(&port); err != nil || port != 51820 {
		t.Fatalf("DB harus tetap port 51820, got %d %v", port, err)
	}
	if response := doRequest(router, "POST", "/api/v1/interfaces",
		strings.Replace(strings.Replace(validInterfaceBody, `"wg0"`, `"wg7"`, 1), `51820`, `51870`, 1), admin, true); response.Code != http.StatusInternalServerError {
		t.Fatalf("POST dengan kernel menolak: %d", response.Code)
	}
	var count int
	if err := app.DB.QueryRow(`SELECT COUNT(*) FROM interfaces WHERE name = 'wg7'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("POST gagal di kernel tidak boleh menyisakan baris: %d %v", count, err)
	}

	if response := doRequest(router, "GET", "/api/v1/interfaces/999", "", admin, false); response.Code != http.StatusNotFound {
		t.Fatalf("GET id tidak ada: %d", response.Code)
	}
}

// TestValidEndpointHost memastikan endpoint yang bisa merusak baris `Endpoint =` ditolak.
func TestValidEndpointHost(t *testing.T) {
	for host, want := range map[string]bool{
		"vpn.example.id": true, "203.0.113.5": true, "a": true,
		"": false, "http://vpn": false, "vpn:51820": false, "vpn example": false, "-vpn.id": false, "vpn..id": false,
		"user@vpn": false, "vpn/path": false,
	} {
		if got := validEndpointHost(host); got != want {
			t.Errorf("%q: want %v got %v", host, want, got)
		}
	}
}
