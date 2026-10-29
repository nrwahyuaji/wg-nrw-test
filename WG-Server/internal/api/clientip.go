// Package api: berkas ini menentukan IP klien sebenarnya untuk rate limit dan audit (DESIGN 7.3).
package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP mengembalikan IP klien. Defaultnya peer TCP (RemoteAddr). X-Forwarded-For hanya dipercaya bila peer
// termasuk trusted (reverse proxy kita sendiri); header dari klien langsung bisa dipalsukan, sehingga diabaikan.
// Bila dipercaya, XFF dibaca dari kanan dan yang dipakai adalah IP pertama yang bukan proxy tepercaya: entri
// kanan ditambahkan oleh proxy kita, entri kiri bisa ditulis bebas oleh klien. Entri rusak menghentikan
// penelusuran dan IP tepercaya terakhir yang dilihat dipakai (lebih aman daripada mempercayai entri kirinya).
func ClientIP(request *http.Request, trusted []netip.Prefix) string {
	peer, ok := parseAddr(request.RemoteAddr)
	if !ok {
		return request.RemoteAddr
	}
	if !isTrusted(peer, trusted) {
		return peer.String()
	}
	client := peer
	hops := strings.Split(strings.Join(request.Header.Values("X-Forwarded-For"), ","), ",")
	for index := len(hops) - 1; index >= 0; index-- {
		hop, ok := parseAddr(strings.TrimSpace(hops[index]))
		if !ok {
			break
		}
		client = hop
		if !isTrusted(hop, trusted) {
			break
		}
	}
	return client.String()
}

// parseAddr menerima "ip", "ip:port", atau "[ipv6]:port" dan menormalkan IPv4-in-IPv6 menjadi IPv4,
// supaya satu klien tidak punya dua kunci rate limit.
func parseAddr(raw string) (netip.Addr, bool) {
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap().WithZone(""), true
}

// isTrusted melaporkan apakah addr termasuk salah satu prefix proxy tepercaya.
func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
