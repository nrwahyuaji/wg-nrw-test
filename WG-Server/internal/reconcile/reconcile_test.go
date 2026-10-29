package reconcile

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/db"
	"wgpanel/internal/secret"
	"wgpanel/internal/wg"
)

// testSecretKey = base64 dari 32 byte nol; hanya untuk uji.
const testSecretKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// fakeKernel mencatat spec terakhir per interface. failNext membuat Apply berikutnya gagal satu kali SETELAH
// mencatat spec — meniru Engine nyata yang bisa gagal di tengah (MTU/alamat sudah berubah, peer belum).
type fakeKernel struct {
	applied  map[string]wg.InterfaceSpec
	removed  []string
	failNext bool
}

func (kernel *fakeKernel) Apply(spec wg.InterfaceSpec) error {
	kernel.applied[spec.Name] = spec
	if kernel.failNext {
		kernel.failNext = false
		return errors.New("kernel menolak")
	}
	return nil
}

func (kernel *fakeKernel) Remove(name string) error {
	delete(kernel.applied, name)
	kernel.removed = append(kernel.removed, name)
	return nil
}

// fixture membuat DB bermigrasi + satu interface wg0 dengan private key terenkripsi.
func fixture(t *testing.T) (*sql.DB, *secret.Box, int64) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "uji.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := db.Migrate(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	box, err := secret.NewBox(testSecretKey, "wg-keys")
	if err != nil {
		t.Fatal(err)
	}
	key, _ := wgtypes.GeneratePrivateKey()
	sealed, _ := box.Seal([]byte(key.String()))
	result, err := database.Exec(`INSERT INTO interfaces (name, address, listen_port, private_key, public_key,
		endpoint_host, created_at, updated_at) VALUES ('wg0', '10.8.0.1/24', 51820, ?, ?, 'vpn', 1, 1)`,
		sealed, key.PublicKey().String())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	return database, box, id
}

// insertPeer menambah peer aktif di transaksi tx (CreatePeer sqlc baru ada di Fase 5).
func insertPeer(ctx context.Context, tx Tx, interfaceID int64, name, address, siteSubnets string) error {
	key, _ := wgtypes.GeneratePrivateKey()
	_, err := tx.Raw.ExecContext(ctx, `INSERT INTO peers (interface_id, name, address, public_key, site_subnets,
		created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, 1)`, interfaceID, name, address, key.PublicKey().String(), siteSubnets)
	return err
}

// TestWithApply: sukses → kernel dan DB sama-sama berisi peer; Apply gagal → DB rollback, kernel dipulihkan
// dari DB lama; fn gagal → Apply tidak dipanggil untuk spec baru.
func TestWithApply(t *testing.T) {
	database, box, id := fixture(t)
	kernel := &fakeKernel{applied: map[string]wg.InterfaceSpec{}}
	reconciler := New(database, kernel, box, "")
	ctx := context.Background()

	err := reconciler.WithApply(ctx, id, func(tx Tx) (int64, error) {
		return 0, insertPeer(ctx, tx, id, "a", "10.8.0.2/32", "192.168.50.0/24")
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := kernel.applied["wg0"]
	if len(spec.Peers) != 1 || len(spec.Peers[0].AllowedIPs) != 2 || spec.ListenPort != 51820 {
		t.Fatalf("spec kernel salah: %+v", spec)
	}

	kernel.failNext = true
	err = reconciler.WithApply(ctx, id, func(tx Tx) (int64, error) {
		return 0, insertPeer(ctx, tx, id, "b", "10.8.0.3/32", "")
	})
	if err == nil || err.Error() != "kernel menolak" {
		t.Fatalf("galat asli harus dikembalikan, got %v", err)
	}
	if count := countPeers(t, database); count != 1 {
		t.Fatalf("DB harus rollback (1 peer), got %d", count)
	}
	if peers := kernel.applied["wg0"].Peers; len(peers) != 1 {
		t.Fatalf("kernel harus dipulihkan ke 1 peer, got %d", len(peers))
	}

	want := errors.New("validasi gagal")
	err = reconciler.WithApply(ctx, id, func(Tx) (int64, error) { return 0, want })
	if !errors.Is(err, want) {
		t.Fatalf("galat fn harus dikembalikan, got %v", err)
	}
}

// TestApplyAll: interface enabled diterapkan, disabled dihapus dari kernel, kunci salah dilaporkan tanpa
// menghentikan interface lain.
func TestApplyAll(t *testing.T) {
	database, box, _ := fixture(t)
	if _, err := database.Exec(`INSERT INTO interfaces (name, address, listen_port, private_key, public_key,
		endpoint_host, enabled, created_at, updated_at) VALUES ('wg1', '10.9.0.1/24', 51821, x'00', 'p', 'vpn', 0, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO interfaces (name, address, listen_port, private_key, public_key,
		endpoint_host, created_at, updated_at) VALUES ('wg2', '10.10.0.1/24', 51822, x'00', 'p', 'vpn', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	kernel := &fakeKernel{applied: map[string]wg.InterfaceSpec{}}
	applied, err := New(database, kernel, box, "").ApplyAll(context.Background())
	if applied != 1 || err == nil {
		t.Fatalf("want 1 diterapkan + galat wg2, got %d %v", applied, err)
	}
	if _, ok := kernel.applied["wg0"]; !ok {
		t.Fatal("wg0 harus diterapkan")
	}
	if len(kernel.removed) != 1 || kernel.removed[0] != "wg1" {
		t.Fatalf("wg1 (disabled) harus di-Remove: %v", kernel.removed)
	}
}

// countPeers menghitung baris peer.
func countPeers(t *testing.T, database *sql.DB) int {
	t.Helper()
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM peers`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// TestWGQuickCopy: salinan wg-quick ditulis 0600 berisi spec yang diterapkan; kernel menolak → berkas tidak
// berubah; tidak ada berkas sementara tersisa.
func TestWGQuickCopy(t *testing.T) {
	database, box, id := fixture(t)
	dir := t.TempDir()
	kernel := &fakeKernel{applied: map[string]wg.InterfaceSpec{}}
	reconciler := New(database, kernel, box, dir)
	ctx := context.Background()
	if err := reconciler.WithApply(ctx, id, func(tx Tx) (int64, error) {
		return 0, insertPeer(ctx, tx, id, "a", "10.8.0.2/32", "192.168.50.0/24")
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "wg0.conf")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("berkas harus ada dengan mode 0600: %v %v", info, err)
	}
	first, _ := os.ReadFile(path)
	spec := kernel.applied["wg0"]
	for _, want := range []string{"PrivateKey = " + spec.PrivateKey.String(), "ListenPort = 51820",
		"PublicKey = " + spec.Peers[0].PublicKey.String(), "AllowedIPs = 10.8.0.2/32, 192.168.50.0/24"} {
		if !strings.Contains(string(first), want) {
			t.Fatalf("salinan tidak memuat %q:\n%s", want, first)
		}
	}

	kernel.failNext = true
	_ = reconciler.WithApply(ctx, id, func(tx Tx) (int64, error) {
		return 0, insertPeer(ctx, tx, id, "b", "10.8.0.3/32", "")
	})
	second, _ := os.ReadFile(path)
	if string(second) != string(first) {
		t.Fatal("salinan tidak boleh berubah setelah kernel menolak spec")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("hanya wg0.conf yang boleh ada, got %d berkas", len(entries))
	}
}
