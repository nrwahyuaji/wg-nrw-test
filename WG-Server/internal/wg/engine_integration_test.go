//go:build wgintegration

// Uji integrasi Engine terhadap kernel sungguhan. WAJIB dijalankan di network namespace terpisah
// (make test-wg) supaya tidak menyentuh jaringan host:
//
//	ip netns add wgtest && ip netns exec wgtest go test -tags wgintegration ./internal/wg/
package wg

import (
	"net/netip"
	"testing"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// TestApplyIdempotent: Apply dua kali memberi hasil sama, peer dinonaktifkan hilang dari kernel,
// subnet berubah mengganti alamat lama, rute site-to-site ditambah lalu dibuang.
func TestApplyIdempotent(t *testing.T) {
	engine, err := NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	defer engine.Remove("wgit0")

	serverKey, _ := wgtypes.GeneratePrivateKey()
	peerA, _ := wgtypes.GeneratePrivateKey()
	peerB, _ := wgtypes.GeneratePrivateKey()
	spec := InterfaceSpec{
		Name: "wgit0", Address: netip.MustParsePrefix("10.66.0.1/24"), ListenPort: 51900, PrivateKey: serverKey,
		Peers: []PeerSpec{
			{PublicKey: peerA.PublicKey(), AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.66.0.2/32")}},
			{PublicKey: peerB.PublicKey(), AllowedIPs: []netip.Prefix{
				netip.MustParsePrefix("10.66.0.3/32"), netip.MustParsePrefix("192.168.77.0/24")}},
		},
	}
	for round := 0; round < 2; round++ {
		if err := engine.Apply(spec); err != nil {
			t.Fatalf("apply ronde %d: %v", round, err)
		}
	}
	status, _ := engine.Status("wgit0")
	if len(status) != 2 {
		t.Fatalf("want 2 peer, got %d", len(status))
	}
	link, _ := netlink.LinkByName("wgit0")
	if countPanelRoutes(t, link) != 1 {
		t.Fatal("rute site-to-site 192.168.77.0/24 harus ada tepat satu")
	}

	// Subnet berubah: IP peer ikut dipindah ke subnet baru. Kalau tidak, IP lama peer jatuh di luar
	// subnet dan syncRoutes (dengan benar) membuat rute /32 untuknya — itulah sebabnya ganti subnet
	// di panel ditolak selama masih ada peer (F-2.3, Tugas 4.2), bukan diserahkan ke kernel.
	spec.Peers = spec.Peers[:1]
	spec.Peers[0].AllowedIPs = []netip.Prefix{netip.MustParsePrefix("10.67.0.2/32")}
	spec.Address = netip.MustParsePrefix("10.67.0.1/24")
	if err := engine.Apply(spec); err != nil {
		t.Fatal(err)
	}
	status, _ = engine.Status("wgit0")
	if len(status) != 1 || status[0].PublicKey != peerA.PublicKey().String() {
		t.Fatalf("hanya peer A yang boleh tersisa: %+v", status)
	}
	addrs, _ := netlink.AddrList(link, netlink.FAMILY_V4)
	if len(addrs) != 1 || addrs[0].IPNet.String() != "10.67.0.1/24" {
		t.Fatalf("alamat harus diganti, got %v", addrs)
	}
	if n := countPanelRoutes(t, link); n != 0 {
		t.Logf("n=%d", n)
		t.Fatal("rute site-to-site harus dibuang setelah peer B hilang")
	}
}

// countPanelRoutes menghitung rute yang ditandai routeProtocol pada link.
func countPanelRoutes(t *testing.T, link netlink.Link) int {
	t.Helper()
	routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, route := range routes {
		if route.Protocol == routeProtocol {
			count++
		}
	}
	return count
}
