package ipam

import (
	"errors"
	"net/netip"
	"testing"
)

// TestNextFree memastikan IP server dilewati, IP terpakai dilewati, dan subnet penuh terdeteksi.
func TestNextFree(t *testing.T) {
	server, err := ParseServerPrefix("10.8.0.1/30") // host: .1 (server) dan .2
	if err != nil {
		t.Fatal(err)
	}
	got, err := NextFree(server, map[netip.Addr]struct{}{})
	if err != nil || got != netip.MustParseAddr("10.8.0.2") {
		t.Fatalf("got %v %v, want 10.8.0.2", got, err)
	}
	used := map[netip.Addr]struct{}{netip.MustParseAddr("10.8.0.2"): {}}
	if _, err := NextFree(server, used); !errors.Is(err, ErrSubnetFull) {
		t.Fatalf("want ErrSubnetFull, got %v", err)
	}
}

// TestValidate menguji tiap jenis penolakan IP manual.
func TestValidate(t *testing.T) {
	server, _ := ParseServerPrefix("10.8.0.1/24")
	used := map[netip.Addr]struct{}{netip.MustParseAddr("10.8.0.5"): {}}
	cases := []struct {
		ip   string
		want error
	}{
		{"10.8.0.9", nil},
		{"10.9.0.9", ErrOutsideSubnet},
		{"10.8.0.0", ErrReserved},
		{"10.8.0.255", ErrReserved},
		{"10.8.0.1", ErrReserved},
		{"10.8.0.5", ErrInUse},
	}
	for _, c := range cases {
		if err := Validate(server, netip.MustParseAddr(c.ip), used); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.ip, err, c.want)
		}
	}
}

// TestParseServerPrefix menolak IPv6, subnet terlalu kecil, dan IP server di alamat network.
func TestParseServerPrefix(t *testing.T) {
	for _, bad := range []string{"fd00::1/64", "10.8.0.1/31", "10.8.0.0/24", "bukan-cidr"} {
		if _, err := ParseServerPrefix(bad); err == nil {
			t.Errorf("%s seharusnya ditolak", bad)
		}
	}
}
