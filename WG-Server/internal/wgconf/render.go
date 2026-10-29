// Package wgconf menyusun berkas konfigurasi WireGuard berformat INI (wg-quick).
// Dipakai untuk dua hal: .conf yang diunduh client, dan wg0.conf di server agar tetap ada setelah reboot.
package wgconf

import (
	"fmt"
	"strings"
)

// ClientConfig adalah isi .conf untuk satu perangkat client.
// PresharedKey, DNS, dan MTU opsional: string kosong / 0 berarti baris tidak ditulis.
type ClientConfig struct {
	PrivateKey          string   // private key client (base64)
	Address             string   // IP client di tunnel, mis. "10.8.0.2/32"
	DNS                 []string // server DNS yang dipakai client saat tunnel aktif
	MTU                 int      // 0 = biarkan client memilih sendiri
	ServerPublicKey     string   // public key interface server
	PresharedKey        string   // opsional, lapisan tambahan tahan-kuantum
	Endpoint            string   // "host:port" publik server, dari WGPANEL_PUBLIC_ENDPOINT
	AllowedIPs          []string // rute yang dilewatkan tunnel, mis. 0.0.0.0/0 (full tunnel)
	PersistentKeepalive int      // detik; 25 umum dipakai agar NAT di sisi client tidak menutup jalur
}

// RenderClient menulis ClientConfig menjadi teks .conf.
// Urutan bagian dan kunci mengikuti contoh resmi wg-quick supaya mudah dibandingkan manual.
func RenderClient(config ClientConfig) string {
	var builder strings.Builder
	builder.WriteString("[Interface]\n")
	fmt.Fprintf(&builder, "PrivateKey = %s\n", config.PrivateKey)
	fmt.Fprintf(&builder, "Address = %s\n", config.Address)
	if len(config.DNS) > 0 {
		fmt.Fprintf(&builder, "DNS = %s\n", strings.Join(config.DNS, ", "))
	}
	if config.MTU > 0 {
		fmt.Fprintf(&builder, "MTU = %d\n", config.MTU)
	}
	builder.WriteString("\n[Peer]\n")
	fmt.Fprintf(&builder, "PublicKey = %s\n", config.ServerPublicKey)
	if config.PresharedKey != "" {
		fmt.Fprintf(&builder, "PresharedKey = %s\n", config.PresharedKey)
	}
	fmt.Fprintf(&builder, "Endpoint = %s\n", config.Endpoint)
	fmt.Fprintf(&builder, "AllowedIPs = %s\n", strings.Join(config.AllowedIPs, ", "))
	if config.PersistentKeepalive > 0 {
		fmt.Fprintf(&builder, "PersistentKeepalive = %d\n", config.PersistentKeepalive)
	}
	return builder.String()
}

// ServerConfig adalah isi salinan wg-quick server (/etc/wireguard/<nama>.conf) untuk pemulihan manual
// (PRD F-2.5): `wg-quick up <nama>` dari berkas ini membangun ulang tunnel tanpa panel.
type ServerConfig struct {
	PrivateKey string // private key server (base64)
	Address    string // CIDR server, mis. 10.8.0.1/24
	ListenPort int
	MTU        int // 0 = baris tidak ditulis (bawaan wg-quick)
	Peers      []ServerPeer
}

// ServerPeer adalah sisi server satu peer aktif.
type ServerPeer struct {
	PublicKey    string
	PresharedKey string   // "" = baris tidak ditulis
	AllowedIPs   []string // IP tunnel /32 + subnet site-to-site
}

// RenderServer menulis ServerConfig sebagai teks wg-quick. Urutan peer = urutan masukan (id naik), sehingga
// berkas stabil byte demi byte dan diff-nya bermakna.
func RenderServer(config ServerConfig) string {
	var builder strings.Builder
	builder.WriteString("[Interface]\n")
	fmt.Fprintf(&builder, "PrivateKey = %s\n", config.PrivateKey)
	fmt.Fprintf(&builder, "Address = %s\n", config.Address)
	fmt.Fprintf(&builder, "ListenPort = %d\n", config.ListenPort)
	if config.MTU > 0 {
		fmt.Fprintf(&builder, "MTU = %d\n", config.MTU)
	}
	for _, peer := range config.Peers {
		builder.WriteString("\n[Peer]\n")
		fmt.Fprintf(&builder, "PublicKey = %s\n", peer.PublicKey)
		if peer.PresharedKey != "" {
			fmt.Fprintf(&builder, "PresharedKey = %s\n", peer.PresharedKey)
		}
		fmt.Fprintf(&builder, "AllowedIPs = %s\n", strings.Join(peer.AllowedIPs, ", "))
	}
	return builder.String()
}
