// Package wg adalah satu-satunya tempat aplikasi menyentuh WireGuard kernel.
// Basis data adalah sumber kebenaran; Engine menyamakan kernel dengan isi basis data (rekonsiliasi),
// sehingga hasil akhirnya sama baik dipanggil sekali maupun berulang kali.
package wg

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// InterfaceSpec adalah keadaan yang diinginkan untuk satu interface WireGuard.
// Hanya peer yang aktif (enabled, belum kedaluwarsa, kuota belum habis) yang dimasukkan pemanggil.
type InterfaceSpec struct {
	Name       string       // nama interface kernel, mis. "wg0"
	Address    netip.Prefix // IP server + ukuran subnet, mis. 10.8.0.1/24
	ListenPort int          // port UDP yang dibuka di VPS
	MTU        int          // 0 = pakai bawaan kernel (1420)
	PrivateKey wgtypes.Key  // private key server yang sudah didekripsi
	Peers      []PeerSpec
}

// PeerSpec adalah sisi server dari satu peer.
// AllowedIPs = IP tunnel peer (/32) ditambah subnet LAN di belakangnya untuk site-to-site.
type PeerSpec struct {
	PublicKey    wgtypes.Key
	PresharedKey *wgtypes.Key // nil = tanpa PSK
	AllowedIPs   []netip.Prefix
	// Endpoint & Keepalive hanya dipakai sisi CLIENT (alat uji cmd/devtools/wgsetconf). Panel tidak mengisinya:
	// server menunggu client menghubungi dan mempelajari endpoint dari handshake.
	Endpoint  string // "host:port"; "" = tidak diset
	Keepalive int    // detik; 0 = mati
}

// PeerStatus adalah pembacaan langsung dari kernel untuk satu peer.
type PeerStatus struct {
	PublicKey     string
	Endpoint      string    // "ip:port" terakhir yang terlihat; kosong bila belum pernah terhubung
	LastHandshake time.Time // nol bila belum pernah handshake
	ReceiveBytes  int64     // penghitung kumulatif sejak interface dibuat (direset saat reboot)
	TransmitBytes int64
}

// defaultMTU dipakai saat spec.MTU = 0. 1420 = 1500 (Ethernet) - 80 (header IPv6+UDP+WireGuard terburuk).
const defaultMTU = 1420

// Engine membungkus klien wgctrl. Buat satu per proses; aman dipakai dari banyak goroutine
// selama pemanggil menserialkan Apply (Reconciler di internal/reconcile memegang mutex global).
type Engine struct {
	client *wgctrl.Client
}

// NewEngine membuka koneksi netlink ke modul WireGuard.
// Gagal bila modul kernel tidak ada atau proses tidak punya CAP_NET_ADMIN.
func NewEngine() (*Engine, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("wg: membuka wgctrl: %w", err)
	}
	return &Engine{client: client}, nil
}

// Close menutup koneksi netlink.
func (engine *Engine) Close() error { return engine.client.Close() }

// Apply menyamakan kernel dengan spec: buat interface bila belum ada, set MTU dan alamat,
// nyalakan, lalu ganti seluruh daftar peer (ReplacePeers) dalam satu panggilan netlink.
// ReplacePeers sengaja dipakai alih-alih diff manual: kernel menerapkannya atomik, dan peer yang
// dihapus/dinonaktifkan di basis data langsung hilang dari kernel tanpa melacak riwayatnya.
func (engine *Engine) Apply(spec InterfaceSpec) error {
	link, err := ensureLink(spec.Name)
	if err != nil {
		return err
	}
	mtu := spec.MTU
	if mtu == 0 {
		mtu = defaultMTU
	}
	if link.Attrs().MTU != mtu {
		if err := netlink.LinkSetMTU(link, mtu); err != nil {
			return fmt.Errorf("wg: set MTU %s: %w", spec.Name, err)
		}
	}
	if err := syncAddress(link, spec.Address); err != nil {
		return err
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("wg: menyalakan %s: %w", spec.Name, err)
	}
	port := spec.ListenPort
	privateKey := spec.PrivateKey
	config := wgtypes.Config{PrivateKey: &privateKey, ListenPort: &port, ReplacePeers: true}
	for _, peer := range spec.Peers {
		peerConfig := wgtypes.PeerConfig{
			PublicKey:         peer.PublicKey,
			PresharedKey:      peer.PresharedKey,
			ReplaceAllowedIPs: true,
			AllowedIPs:        toIPNets(peer.AllowedIPs),
		}
		if peer.Endpoint != "" {
			endpoint, err := net.ResolveUDPAddr("udp", peer.Endpoint)
			if err != nil {
				return fmt.Errorf("wg: endpoint %s: %w", peer.Endpoint, err)
			}
			peerConfig.Endpoint = endpoint
		}
		if peer.Keepalive > 0 {
			interval := time.Duration(peer.Keepalive) * time.Second
			peerConfig.PersistentKeepaliveInterval = &interval
		}
		config.Peers = append(config.Peers, peerConfig)
	}
	if err := engine.client.ConfigureDevice(spec.Name, config); err != nil {
		return fmt.Errorf("wg: konfigurasi %s: %w", spec.Name, err)
	}
	return syncRoutes(link, spec)
}

// Status membaca keadaan langsung semua peer di interface. Dipakai halaman status (polling 5 dtk)
// dan sampler trafik (tiap 60 dtk).
func (engine *Engine) Status(name string) ([]PeerStatus, error) {
	device, err := engine.client.Device(name)
	if err != nil {
		return nil, fmt.Errorf("wg: membaca %s: %w", name, err)
	}
	result := make([]PeerStatus, 0, len(device.Peers))
	for _, peer := range device.Peers {
		status := PeerStatus{
			PublicKey:     peer.PublicKey.String(),
			LastHandshake: peer.LastHandshakeTime,
			ReceiveBytes:  peer.ReceiveBytes,
			TransmitBytes: peer.TransmitBytes,
		}
		if peer.Endpoint != nil {
			status.Endpoint = peer.Endpoint.String()
		}
		result = append(result, status)
	}
	return result, nil
}

// InterfaceState adalah pembacaan langsung keadaan satu interface di kernel (untuk pemeriksaan kesehatan
// dan uji). Exists=false berarti link tidak ada sama sekali; field lain lalu bernilai nol.
type InterfaceState struct {
	Exists     bool
	Up         bool
	ListenPort int
	PublicKey  string
	Peers      []PeerStatus
}

// Inspect membaca link + device WireGuard bernama name. Link yang tidak ada bukan galat (Exists=false),
// supaya pemanggil bisa membedakan "belum dibuat" dari "gagal membaca".
func (engine *Engine) Inspect(name string) (InterfaceState, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		var notFound netlink.LinkNotFoundError
		if errors.As(err, &notFound) {
			return InterfaceState{}, nil
		}
		return InterfaceState{}, err
	}
	device, err := engine.client.Device(name)
	if err != nil {
		return InterfaceState{}, fmt.Errorf("wg: membaca %s: %w", name, err)
	}
	peers, err := engine.Status(name)
	if err != nil {
		return InterfaceState{}, err
	}
	return InterfaceState{
		Exists:     true,
		Up:         link.Attrs().Flags&net.FlagUp != 0,
		ListenPort: device.ListenPort,
		PublicKey:  device.PublicKey.String(),
		Peers:      peers,
	}, nil
}

// Remove menghapus interface dari kernel (dipakai saat interface dihapus di panel, Fase 15).
// Interface yang memang tidak ada tidak dianggap galat.
func (engine *Engine) Remove(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		var notFound netlink.LinkNotFoundError
		if errors.As(err, &notFound) {
			return nil
		}
		return err
	}
	return netlink.LinkDel(link)
}

// ensureLink mengambil interface bernama name, atau membuatnya bertipe wireguard bila belum ada.
// Bila nama itu sudah dipakai interface jenis lain, Apply ditolak agar tidak merusak jaringan host.
func ensureLink(name string) (netlink.Link, error) {
	link, err := netlink.LinkByName(name)
	if err == nil {
		if link.Type() != "wireguard" {
			return nil, fmt.Errorf("wg: %s sudah ada tetapi bertipe %s, bukan wireguard", name, link.Type())
		}
		return link, nil
	}
	var notFound netlink.LinkNotFoundError
	if !errors.As(err, &notFound) {
		return nil, err
	}
	created := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: name}}
	if err := netlink.LinkAdd(created); err != nil {
		return nil, fmt.Errorf("wg: membuat %s: %w", name, err)
	}
	return netlink.LinkByName(name)
}

// syncAddress memastikan interface hanya memegang satu alamat IPv4 = spec.Address.
// Alamat lama (mis. setelah subnet diubah) dihapus supaya tidak ada dua rute subnet yang bersaing.
func syncAddress(link netlink.Link, want netip.Prefix) error {
	current, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	for _, addr := range current {
		if addr.IPNet.String() != want.String() {
			if err := netlink.AddrDel(link, &addr); err != nil {
				return fmt.Errorf("wg: hapus alamat lama %s: %w", addr.IPNet, err)
			}
		}
	}
	parsed, err := netlink.ParseAddr(want.String())
	if err != nil {
		return err
	}
	return netlink.AddrReplace(link, parsed)
}

// syncRoutes menambahkan rute kernel untuk AllowedIPs peer yang berada di LUAR subnet interface
// (subnet LAN site-to-site). Subnet interface sendiri sudah dirutekan otomatis oleh alamatnya.
// Rute lama yang tidak lagi dipakai dibuang, sehingga hasilnya sama setiap kali dipanggil.
func syncRoutes(link netlink.Link, spec InterfaceSpec) error {
	wanted := map[string]netip.Prefix{}
	for _, peer := range spec.Peers {
		for _, prefix := range peer.AllowedIPs {
			if !spec.Address.Masked().Contains(prefix.Addr()) {
				wanted[prefix.Masked().String()] = prefix.Masked()
			}
		}
	}
	existing, err := netlink.RouteList(link, netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	for _, route := range existing {
		if route.Dst == nil || route.Protocol != routeProtocol {
			continue // rute milik kernel/alamat interface, bukan milik panel
		}
		key := route.Dst.String()
		if _, keep := wanted[key]; keep {
			delete(wanted, key)
			continue
		}
		if err := netlink.RouteDel(&route); err != nil {
			return fmt.Errorf("wg: hapus rute %s: %w", key, err)
		}
	}
	for _, prefix := range wanted {
		destination := toIPNets([]netip.Prefix{prefix})[0]
		route := netlink.Route{LinkIndex: link.Attrs().Index, Dst: &destination, Protocol: routeProtocol}
		if err := netlink.RouteReplace(&route); err != nil {
			return fmt.Errorf("wg: tambah rute %s: %w", prefix, err)
		}
	}
	return nil
}

// routeProtocol menandai rute buatan panel (nilai bebas di rentang 'static' admin, lihat
// /etc/iproute2/rt_protos). Dengan penanda ini syncRoutes tidak pernah menghapus rute milik pihak lain.
const routeProtocol netlink.RouteProtocol = 51

// toIPNets mengubah netip.Prefix ke net.IPNet yang diminta API wgctrl/netlink.
func toIPNets(prefixes []netip.Prefix) []net.IPNet {
	result := make([]net.IPNet, 0, len(prefixes))
	for _, prefix := range prefixes {
		masked := prefix.Masked()
		result = append(result, net.IPNet{
			IP:   net.IP(masked.Addr().AsSlice()),
			Mask: net.CIDRMask(masked.Bits(), masked.Addr().BitLen()),
		})
	}
	return result
}
