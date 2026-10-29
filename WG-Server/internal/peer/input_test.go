package peer

import (
	"net/netip"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// testContext: server 10.8.0.1/24, IP .2 terpakai, peer lain membawa LAN 192.168.50.0/24.
func testContext() Context {
	return Context{
		Server:        netip.MustParsePrefix("10.8.0.1/24"),
		UsedAddresses: map[netip.Addr]struct{}{netip.MustParseAddr("10.8.0.2"): {}},
		OtherSites:    []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")},
	}
}

// TestValidate: alokasi otomatis, IP manual, bentrok (conflict), dan tiap jenis penolakan.
func TestValidate(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	input, fields, _ := Validate(Request{Name: " laptop ", PublicKey: key.PublicKey().String(),
		ClientAllowedIPs: []string{"10.8.0.0/24", "10.8.0.9/24"}, SiteSubnets: []string{"192.168.60.1/24"}}, testContext())
	if len(fields) != 0 {
		t.Fatalf("harus lolos: %+v", fields)
	}
	if input.Name != "laptop" || input.Address != "10.8.0.3/32" || input.ClientAllowedIPs != "10.8.0.0/24" ||
		input.SiteSubnets != "192.168.60.0/24" || input.PublicKey != key.PublicKey().String() {
		t.Fatalf("hasil salah: %+v", input)
	}
	if input, _, _ := Validate(Request{Name: "x", Address: "10.8.0.9/32"}, testContext()); input.Address != "10.8.0.9/32" {
		t.Fatalf("IP manual: %+v", input)
	}

	if _, fields, conflict := Validate(Request{Name: "x", Address: "10.8.0.2"}, testContext()); !conflict || len(fields) != 1 {
		t.Fatalf("IP terpakai harus conflict: %v %+v", conflict, fields)
	}
	cases := map[string]Request{
		"nama kosong":       {Name: " "},
		"IP luar subnet":    {Name: "x", Address: "10.9.0.5"},
		"IP server":         {Name: "x", Address: "10.8.0.1"},
		"IP bukan /32":      {Name: "x", Address: "10.8.0.0/24"},
		"public key rusak":  {Name: "x", PublicKey: "bukan-kunci"},
		"allowed rusak":     {Name: "x", ClientAllowedIPs: []string{"0.0.0.0/0", "salah"}},
		"site = subnet VPN": {Name: "x", SiteSubnets: []string{"10.8.0.0/25"}},
		"site bentrok":      {Name: "x", SiteSubnets: []string{"192.168.50.128/25"}},
		"site saling iris":  {Name: "x", SiteSubnets: []string{"172.16.0.0/16", "172.16.5.0/24"}},
	}
	for name, request := range cases {
		if _, fields, conflict := Validate(request, testContext()); len(fields) == 0 || conflict {
			t.Errorf("%s: harus 422, got fields=%+v conflict=%v", name, fields, conflict)
		}
	}
	many := make([]string, MaxAllowedIPs+1)
	for i := range many {
		many[i] = "10.0.0.0/8"
	}
	if _, fields, _ := Validate(Request{Name: "x", ClientAllowedIPs: many}, testContext()); len(fields) == 0 {
		t.Fatal("lebih dari 32 AllowedIPs harus ditolak")
	}

	full := testContext()
	full.Server = netip.MustParsePrefix("10.8.0.1/30") // host: .1 server, .2 terpakai → penuh
	if _, fields, _ := Validate(Request{Name: "x"}, full); len(fields) != 1 || fields[0].Field != "address" {
		t.Fatalf("subnet penuh: %+v", fields)
	}
}
