//go:build wgintegration

// Uji integrasi Reconciler terhadap kernel sungguhan. WAJIB lewat `make test-wg` (netns sekali pakai).
package reconcile

import (
	"context"
	"net/netip"
	"testing"

	"github.com/vishvananda/netlink"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/wg"
)

// TestReconcileKernel (PLAN Tugas 4.1): fn sukses → kernel punya peer baru; fn sukses tetapi Apply gagal
// (port bentrok dengan interface lain) → DB tidak berubah dan kernel = keadaan lama; ApplyAll setelah
// Remove manual → interface kembali (simulasi reboot, DoD #3).
func TestReconcileKernel(t *testing.T) {
	database, box, id := fixture(t)
	engine, err := wg.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = engine.Close() }()
	defer func() { _ = engine.Remove("wg0"); _ = engine.Remove("wgblok") }()
	reconciler := New(database, engine, box, "")
	ctx := context.Background()

	if _, err := reconciler.ApplyAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.WithApply(ctx, id, func(tx Tx) (int64, error) {
		return 0, insertPeer(ctx, tx, id, "a", "10.8.0.2/32", "")
	}); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Inspect("wg0")
	if err != nil || !state.Exists || !state.Up || state.ListenPort != 51820 || len(state.Peers) != 1 {
		t.Fatalf("setelah tambah peer: %+v %v", state, err)
	}

	// Interface lain memegang port 51830; wg0 diminta pindah ke port itu → kernel menolak (address in use).
	blocker := wg.InterfaceSpec{Name: "wgblok", Address: netip.MustParsePrefix("10.99.0.1/24"), ListenPort: 51830}
	blocker.PrivateKey = generateKey(t)
	if err := engine.Apply(blocker); err != nil {
		t.Fatal(err)
	}
	err = reconciler.WithApply(ctx, id, func(tx Tx) (int64, error) {
		// MTU diterapkan Engine SEBELUM ConfigureDevice, jadi kegagalan port meninggalkan kernel setengah
		// berubah (MTU 1300) — hanya pemulihan dari DB yang mengembalikannya ke 1420.
		if _, err := tx.Raw.ExecContext(ctx, `UPDATE interfaces SET listen_port = 51830, mtu = 1300 WHERE id = ?`, id); err != nil {
			return 0, err
		}
		return 0, insertPeer(ctx, tx, id, "b", "10.8.0.3/32", "")
	})
	if err == nil {
		t.Fatal("Apply dengan port bentrok seharusnya gagal")
	}
	var port int
	if err := database.QueryRow(`SELECT listen_port FROM interfaces WHERE id = ?`, id).Scan(&port); err != nil || port != 51820 {
		t.Fatalf("DB harus rollback ke port 51820, got %d %v", port, err)
	}
	if count := countPeers(t, database); count != 1 {
		t.Fatalf("DB harus rollback ke 1 peer, got %d", count)
	}
	state, _ = engine.Inspect("wg0")
	if state.ListenPort != 51820 || len(state.Peers) != 1 || linkMTU(t, "wg0") != 1420 {
		t.Fatalf("kernel harus kembali ke keadaan lama: %+v mtu=%d", state, linkMTU(t, "wg0"))
	}

	// Simulasi reboot: interface hilang dari kernel, ApplyAll membangunnya ulang dari DB.
	if err := engine.Remove("wg0"); err != nil {
		t.Fatal(err)
	}
	if state, _ := engine.Inspect("wg0"); state.Exists {
		t.Fatal("wg0 seharusnya sudah terhapus")
	}
	if applied, err := reconciler.ApplyAll(ctx); err != nil || applied != 1 {
		t.Fatalf("ApplyAll: %d %v", applied, err)
	}
	state, _ = engine.Inspect("wg0")
	if !state.Exists || state.ListenPort != 51820 || len(state.Peers) != 1 {
		t.Fatalf("wg0 harus kembali utuh: %+v", state)
	}
}

// generateKey membuat private key acak untuk interface pengganjal.
func generateKey(t *testing.T) wgtypes.Key {
	t.Helper()
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// linkMTU membaca MTU link langsung dari kernel (InterfaceState sengaja tidak memuatnya).
func linkMTU(t *testing.T, name string) int {
	t.Helper()
	link, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatal(err)
	}
	return link.Attrs().MTU
}
