package api

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

// TestClientIP: XFF dari peer tidak tepercaya diabaikan, XFF berantai diambil dari kanan melewati proxy
// tepercaya, entri palsu di kiri tidak dipakai, dan entri rusak tidak membuat klien bisa memilih IP.
func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.251.0.0/30"), netip.MustParsePrefix("172.16.0.0/12")}
	cases := []struct {
		name, remote, forwarded, want string
	}{
		{"tanpa XFF", "203.0.113.7:5555", "", "203.0.113.7"},
		{"XFF dari peer tidak tepercaya diabaikan", "203.0.113.7:5555", "1.2.3.4", "203.0.113.7"},
		{"satu proxy tepercaya", "10.251.0.1:4000", "198.51.100.9", "198.51.100.9"},
		{"berantai: klien palsu di kiri diabaikan", "10.251.0.1:4000", "6.6.6.6, 198.51.100.9, 172.16.5.5", "198.51.100.9"},
		{"semua hop tepercaya: pakai paling kiri", "10.251.0.1:4000", "172.16.0.9, 172.16.5.5", "172.16.0.9"},
		{"entri rusak menghentikan penelusuran", "10.251.0.1:4000", "6.6.6.6, bukan-ip, 172.16.5.5", "172.16.5.5"},
		{"tepercaya tanpa XFF", "10.251.0.1:4000", "", "10.251.0.1"},
		{"IPv4-in-IPv6 dinormalkan", "[::ffff:203.0.113.7]:5555", "", "203.0.113.7"},
		{"IPv6 dengan port di XFF", "10.251.0.1:4000", "[2001:db8::1]:443", "2001:db8::1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/", nil)
			request.RemoteAddr = testCase.remote
			if testCase.forwarded != "" {
				request.Header.Set("X-Forwarded-For", testCase.forwarded)
			}
			if got := ClientIP(request, trusted); got != testCase.want {
				t.Fatalf("ClientIP = %q, want %q", got, testCase.want)
			}
		})
	}

	// Beberapa baris header XFF digabung sesuai urutan kedatangan.
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "10.251.0.1:4000"
	request.Header.Add("X-Forwarded-For", "6.6.6.6")
	request.Header.Add("X-Forwarded-For", "198.51.100.9")
	if got := ClientIP(request, trusted); got != "198.51.100.9" {
		t.Fatalf("XFF multi-baris = %q", got)
	}
}
