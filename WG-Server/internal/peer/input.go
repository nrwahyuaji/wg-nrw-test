// Package peer memegang aturan domain peer yang dipakai lebih dari satu jalur (API admin sekarang, portal
// user di Fase 17): validasi input dan penyusunan config client. Paket ini tidak menyentuh HTTP maupun kernel.
package peer

import (
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"wgpanel/internal/ipam"
)

// Batas input. maxAllowedIPs = 32 sesuai PLAN 5.1; nama dibatasi agar muat di daftar dan nama berkas .conf.
const (
	MaxAllowedIPs = 32
	MaxSiteSubnet = 32
	maxNameRunes  = 64
	maxNoteRunes  = 500
)

// FieldError adalah satu galat validasi per field (dikirim ke frontend apa adanya).
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Request adalah isian peer dari pengguna, sebelum divalidasi.
type Request struct {
	Name             string
	Note             string
	Address          string   // "" = alokasi otomatis
	PublicKey        string   // "" = keypair dibuat server
	ClientAllowedIPs []string // kosong = ikut interface
	SiteSubnets      []string // LAN di belakang peer (site-to-site)
	ExpiresAt        *int64   // ISO-8601 WITA → unix UTC; nil = tanpa batas
	QuotaBytes       *int64   // GiB, ≥ 0; nil = tanpa kuota
	QuotaPeriod      string   // "none" atau "monthly"; kosong = tanpa kuota
}

// Context adalah keadaan yang dibutuhkan validasi: subnet server, IP yang sudah dipakai (selain peer yang
// sedang diubah), dan subnet site-to-site milik peer lain.
type Context struct {
	Server        netip.Prefix
	UsedAddresses map[netip.Addr]struct{}
	OtherSites    []netip.Prefix
}

// Input adalah hasil validasi, siap ditulis ke DB. Address selalu /32. ExpiresAt/QuotaBytes nil
// berarti "tanpa batas"; QuotaPeriod kosong berarti tanpa kuota (F-9.1).
type Input struct {
	Name, Note, Address, PublicKey, ClientAllowedIPs, SiteSubnets string
	ExpiresAt                                                     *int64 // unix UTC; nil = tanpa batas
	QuotaBytes                                                    *int64 // GiB, ≥ 0; nil = tanpa kuota
	QuotaPeriod                                                   string // "none" atau "monthly"
}

// Validate memeriksa seluruh field sekaligus dan mengembalikan semua galat, supaya pengguna memperbaiki form
// dalam satu kali kirim. Alamat kosong diisi IP kosong pertama (ipam.NextFree).
// conflict=true berarti galat alamat adalah bentrok dengan peer lain (409), bukan validasi (422).
func Validate(request Request, context Context) (input Input, fields []FieldError, conflict bool) {
	add := func(field, message string) { fields = append(fields, FieldError{Field: field, Message: message}) }

	input.Name = strings.TrimSpace(request.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > maxNameRunes || strings.ContainsAny(input.Name, "\r\n\t") {
		add("name", "nama wajib diisi, maksimal 64 karakter, tanpa baris baru")
	}
	input.Note = strings.TrimSpace(request.Note)
	if utf8.RuneCountInString(input.Note) > maxNoteRunes {
		add("note", "catatan maksimal 500 karakter")
	}

	address, addressConflict, message := pickAddress(strings.TrimSpace(request.Address), context)
	if message != "" {
		add("address", message)
		conflict = addressConflict
	} else {
		input.Address = netip.PrefixFrom(address, 32).String()
	}

	if key := strings.TrimSpace(request.PublicKey); key != "" {
		parsed, err := wgtypes.ParseKey(key)
		if err != nil {
			add("public_key", "public key harus base64 kunci WireGuard (44 karakter)")
		} else {
			input.PublicKey = parsed.String()
		}
	}

	allowed, ok := parseCIDRList(request.ClientAllowedIPs, MaxAllowedIPs)
	if !ok {
		add("client_allowed_ips", "AllowedIPs harus daftar CIDR, maksimal 32")
	}
	input.ClientAllowedIPs = joinPrefixes(allowed)

	sites, ok := parseCIDRList(request.SiteSubnets, MaxSiteSubnet)
	if !ok {
		add("site_subnets", "subnet site-to-site harus daftar CIDR IPv4, maksimal 32")
	} else if message := checkSites(sites, context); message != "" {
		add("site_subnets", message)
	}
	input.SiteSubnets = joinPrefixes(sites)

	// expires_at: masa berlaku harus di masa depan saat peer dibuat (F-9.1).
	if request.ExpiresAt != nil && *request.ExpiresAt <= time.Now().Unix() {
		add("expires_at", "tanggal masa berlaku sudah kedaluwarsa")
	}
	// quota_bytes: ≥ 0, input UI dalam GiB (F-9.1).
	if request.QuotaBytes != nil && *request.QuotaBytes < 0 {
		add("quota_bytes", "kuota harus bilangan bulat ≥ 0")
	}
	// quota_period: "none" atau "monthly" (F-9.1).
	if request.QuotaPeriod != "" && request.QuotaPeriod != "none" && request.QuotaPeriod != "monthly" {
		add("quota_period", "kuota_period harus \"none\" atau \"monthly\"")
	}

	input.ExpiresAt = request.ExpiresAt
	input.QuotaBytes = request.QuotaBytes
	input.QuotaPeriod = request.QuotaPeriod
	if len(fields) > 1 {
		conflict = false // ada galat validasi lain: kirim 422 supaya semua field terlihat
	}
	return input, fields, conflict
}

// pickAddress memilih IP otomatis atau memvalidasi IP manual. "10.8.0.5" dan "10.8.0.5/32" sama-sama diterima.
func pickAddress(raw string, context Context) (addr netip.Addr, conflict bool, message string) {
	if raw == "" {
		next, err := ipam.NextFree(context.Server, context.UsedAddresses)
		if err != nil {
			return netip.Addr{}, false, "subnet interface penuh, tidak ada IP kosong"
		}
		return next, false, ""
	}
	if prefix, err := netip.ParsePrefix(raw); err == nil {
		if prefix.Bits() != 32 {
			return netip.Addr{}, false, "alamat peer harus satu IP (/32)"
		}
		raw = prefix.Addr().String()
	}
	parsed, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, false, "alamat harus IPv4, mis. 10.8.0.5"
	}
	switch err := ipam.Validate(context.Server, parsed, context.UsedAddresses); err {
	case nil:
		return parsed, false, ""
	case ipam.ErrInUse:
		return netip.Addr{}, true, "IP sudah dipakai peer lain"
	case ipam.ErrReserved:
		return netip.Addr{}, false, "IP adalah alamat network, broadcast, atau milik server"
	default:
		return netip.Addr{}, false, "IP di luar subnet interface"
	}
}

// checkSites menolak subnet LAN yang beririsan dengan subnet interface, dengan subnet LAN peer lain, atau
// dengan sesamanya: dua rute untuk tujuan yang sama membuat kernel memilih acak (F-3.8).
func checkSites(sites []netip.Prefix, context Context) string {
	for index, site := range sites {
		if !site.Addr().Is4() {
			return "subnet site-to-site hanya IPv4"
		}
		if site.Overlaps(context.Server.Masked()) {
			return "subnet " + site.String() + " beririsan dengan subnet interface"
		}
		for _, other := range context.OtherSites {
			if site.Overlaps(other) {
				return "subnet " + site.String() + " beririsan dengan subnet LAN peer lain (" + other.String() + ")"
			}
		}
		for _, sibling := range sites[:index] {
			if site.Overlaps(sibling) {
				return "subnet " + site.String() + " beririsan dengan " + sibling.String()
			}
		}
	}
	return ""
}

// parseCIDRList membaca daftar CIDR, menormalkan ke alamat network, dan membuang duplikat. ok=false bila ada
// item rusak atau melebihi batas.
func parseCIDRList(items []string, limit int) ([]netip.Prefix, bool) {
	if len(items) > limit {
		return nil, false
	}
	seen := map[netip.Prefix]struct{}{}
	result := make([]netip.Prefix, 0, len(items))
	for _, item := range items {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return nil, false
		}
		prefix = prefix.Masked()
		if _, dup := seen[prefix]; dup {
			continue
		}
		seen[prefix] = struct{}{}
		result = append(result, prefix)
	}
	return result, true
}

// ParseCIDRColumn membaca kolom DB berisi CIDR dipisah koma; item rusak dilewati (kolom ditulis paket ini).
func ParseCIDRColumn(raw string) []netip.Prefix {
	var result []netip.Prefix
	for _, item := range strings.Split(raw, ",") {
		if prefix, err := netip.ParsePrefix(strings.TrimSpace(item)); err == nil {
			result = append(result, prefix)
		}
	}
	return result
}

// joinPrefixes menyusun kolom DB "a,b,c".
func joinPrefixes(prefixes []netip.Prefix) string {
	parts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		parts = append(parts, prefix.String())
	}
	return strings.Join(parts, ",")
}
