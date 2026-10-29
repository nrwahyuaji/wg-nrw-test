// Command wgsetconf adalah alat uji dev (bukan bagian panel): memasang isi berkas .conf client ke interface
// WireGuard yang SUDAH ada, sebagai pengganti `wg setconf` + `ip addr` karena host dev tidak punya wireguard-tools.
// Hanya dijalankan di netns uji wgcli oleh scripts/e2e-handshake.sh. Pemakaian:
//
//	ip netns exec wgcli wgsetconf <interface> <berkas.conf>
//
// Kernel disentuh lewat internal/wg (Engine.Apply), sesuai aturan AGENTS.md §2.
package main

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/wg"
)

// main membaca .conf lalu menerapkannya; galat apa pun → exit 1 dengan pesan.
func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "pakai: wgsetconf <interface> <berkas.conf>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "wgsetconf:", err)
		os.Exit(1)
	}
}

// run mengurai .conf client (satu [Interface], satu [Peer]) dan menerapkannya sebagai InterfaceSpec.
// Sisi client tidak membuka port tetap (ListenPort 0 = acak) dan menambahkan Endpoint + keepalive peer.
func run(name, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	values := map[string]string{}
	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[]")
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("baris tidak dikenal: %q", line)
		}
		values[section+"."+strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	private, err := wgtypes.ParseKey(values["Interface.PrivateKey"])
	if err != nil {
		return fmt.Errorf("kunci PrivateKey: %w", err)
	}
	address, err := netip.ParsePrefix(values["Interface.Address"])
	if err != nil {
		return fmt.Errorf("kolom Address: %w", err)
	}
	server, err := wgtypes.ParseKey(values["Peer.PublicKey"])
	if err != nil {
		return fmt.Errorf("kunci PublicKey peer: %w", err)
	}
	peer := wg.PeerSpec{PublicKey: server, Endpoint: values["Peer.Endpoint"], Keepalive: atoiOrZero(values["Peer.PersistentKeepalive"])}
	if psk := values["Peer.PresharedKey"]; psk != "" {
		parsed, err := wgtypes.ParseKey(psk)
		if err != nil {
			return fmt.Errorf("kunci PresharedKey: %w", err)
		}
		peer.PresharedKey = &parsed
	}
	for _, item := range strings.Split(values["Peer.AllowedIPs"], ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return fmt.Errorf("kolom AllowedIPs: %w", err)
		}
		peer.AllowedIPs = append(peer.AllowedIPs, prefix)
	}
	engine, err := wg.NewEngine()
	if err != nil {
		return err
	}
	defer func() { _ = engine.Close() }()
	return engine.Apply(wg.InterfaceSpec{Name: name, Address: address, PrivateKey: private, Peers: []wg.PeerSpec{peer}})
}

// atoiOrZero membaca angka keepalive; kosong/rusak = 0 (tanpa keepalive).
func atoiOrZero(raw string) int {
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil {
		return 0
	}
	return value
}
