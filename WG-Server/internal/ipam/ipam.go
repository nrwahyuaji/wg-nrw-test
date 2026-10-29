// Package ipam membagikan alamat IP client di dalam subnet sebuah interface WireGuard.
// Hanya IPv4 pada versi 1 (lihat PRD §2 non-tujuan).
package ipam

import (
	"encoding/binary"
	"errors"
	"net/netip"
)

// Galat domain IPAM. Handler API memetakan galat ini ke 422 dengan pesan berbahasa Indonesia.
var (
	ErrNotIPv4        = errors.New("ipam: hanya subnet IPv4 yang didukung")
	ErrSubnetTooSmall = errors.New("ipam: subnet terlalu kecil (minimal /30)")
	ErrSubnetFull     = errors.New("ipam: subnet penuh, tidak ada IP kosong")
	ErrOutsideSubnet  = errors.New("ipam: IP di luar subnet interface")
	ErrReserved       = errors.New("ipam: IP adalah alamat network, broadcast, atau milik server")
	ErrInUse          = errors.New("ipam: IP sudah dipakai peer lain")
)

// ParseServerPrefix membaca alamat server berformat CIDR, mis. "10.8.0.1/24".
// Bagian IP adalah alamat server di tunnel; bagian prefix adalah ukuran subnet client.
// Return galat bila bukan IPv4, lebih kecil dari /30, atau IP server jatuh di network/broadcast.
func ParseServerPrefix(cidr string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return netip.Prefix{}, err
	}
	if !prefix.Addr().Is4() {
		return netip.Prefix{}, ErrNotIPv4
	}
	// /31 dan /32 tidak menyisakan IP untuk client sama sekali.
	if prefix.Bits() > 30 {
		return netip.Prefix{}, ErrSubnetTooSmall
	}
	network := prefix.Masked()
	if prefix.Addr() == network.Addr() || prefix.Addr() == broadcastAddr(network) {
		return netip.Prefix{}, ErrReserved
	}
	return prefix, nil
}

// NextFree mengembalikan IP client pertama yang kosong di subnet server.
// used berisi IP yang sudah dipegang peer pada interface yang sama.
// Pencarian linear sengaja dipilih: subnet terbesar yang masuk akal (/16) hanya 65 ribu langkah
// dan fungsi ini dipanggil saat admin menambah peer, bukan di jalur panas.
func NextFree(server netip.Prefix, used map[netip.Addr]struct{}) (netip.Addr, error) {
	network := server.Masked()
	broadcast := broadcastAddr(network)
	for addr := network.Addr().Next(); addr.Less(broadcast); addr = addr.Next() {
		if addr == server.Addr() {
			continue
		}
		if _, taken := used[addr]; taken {
			continue
		}
		return addr, nil
	}
	return netip.Addr{}, ErrSubnetFull
}

// Validate memeriksa IP yang diisi admin secara manual: harus di dalam subnet, bukan
// network/broadcast/IP server, dan belum dipakai peer lain.
func Validate(server netip.Prefix, addr netip.Addr, used map[netip.Addr]struct{}) error {
	network := server.Masked()
	if !addr.Is4() || !network.Contains(addr) {
		return ErrOutsideSubnet
	}
	if addr == network.Addr() || addr == broadcastAddr(network) || addr == server.Addr() {
		return ErrReserved
	}
	if _, taken := used[addr]; taken {
		return ErrInUse
	}
	return nil
}

// broadcastAddr menghitung alamat terakhir subnet IPv4 (semua bit host = 1).
func broadcastAddr(network netip.Prefix) netip.Addr {
	octets := network.Addr().As4()
	hostMask := uint32(1)<<(32-network.Bits()) - 1
	binary.BigEndian.PutUint32(octets[:], binary.BigEndian.Uint32(octets[:])|hostMask)
	return netip.AddrFrom4(octets)
}
