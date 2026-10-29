// Package peer: berkas ini menyusun config client satu peer. Satu-satunya tempat yang memetakan baris DB ke
// wgconf.ClientConfig, dipakai unduhan .conf, QR, tautan berbagi (Fase 10), dan portal (Fase 17), supaya keempat
// jalur selalu menghasilkan isi yang sama.
package peer

import (
	"net"
	"strconv"
	"strings"

	"wgpanel/internal/secret"
	"wgpanel/internal/sqlc"
	"wgpanel/internal/wgconf"
)

// PrivateKeyPlaceholder ditulis bila private key tidak pernah ada di server (admin menempel public key,
// PRD F-3.2). Pengguna mengisinya sendiri di perangkat.
const PrivateKeyPlaceholder = "<ISI_DI_PERANGKAT>"

// BuildClientConfig menyusun config client. Endpoint = endpoint_host:listen_port (IPv6 dibungkus []),
// AllowedIPs = milik peer bila diisi, selain itu milik interface. Galat bila kunci tersimpan tidak bisa dibuka
// (WGPANEL_SECRET_KEY berbeda dari saat kunci dibuat).
func BuildClientConfig(iface sqlc.Interface, row sqlc.Peer, box *secret.Box) (wgconf.ClientConfig, error) {
	config := wgconf.ClientConfig{
		PrivateKey:          PrivateKeyPlaceholder,
		Address:             row.Address,
		DNS:                 splitColumn(iface.ClientDns),
		MTU:                 int(iface.Mtu),
		ServerPublicKey:     iface.PublicKey,
		Endpoint:            net.JoinHostPort(iface.EndpointHost, strconv.FormatInt(iface.ListenPort, 10)),
		AllowedIPs:          splitColumn(row.ClientAllowedIps),
		PersistentKeepalive: int(iface.ClientKeepalive),
	}
	if len(config.AllowedIPs) == 0 {
		config.AllowedIPs = splitColumn(iface.ClientAllowedIps)
	}
	if len(row.PrivateKey) > 0 {
		plain, err := box.Open(row.PrivateKey)
		if err != nil {
			return wgconf.ClientConfig{}, err
		}
		config.PrivateKey = string(plain)
	}
	if len(row.PresharedKey) > 0 {
		plain, err := box.Open(row.PresharedKey)
		if err != nil {
			return wgconf.ClientConfig{}, err
		}
		config.PresharedKey = string(plain)
	}
	return config, nil
}

// SafeFileName membuat nama berkas .conf dari nama peer: hanya [A-Za-z0-9._-], karakter lain → '-', maks 40
// karakter, tidak kosong. Nama ini masuk header Content-Disposition, jadi tanda kutip/CRLF tidak boleh lolos.
func SafeFileName(name string) string {
	var builder strings.Builder
	for _, char := range name {
		if builder.Len() >= 40 {
			break
		}
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			char == '.' || char == '_' || char == '-' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	result := strings.Trim(builder.String(), ".")
	if result == "" {
		return "peer"
	}
	return result
}

// splitColumn memecah kolom dipisah koma; kolom kosong → nil (baris opsional tidak ditulis).
func splitColumn(raw string) []string {
	var result []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
