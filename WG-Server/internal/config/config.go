// Package config membaca konfigurasi wgpanel dari variabel lingkungan dan memvalidasinya sekali saat start.
// Sengaja hanya env (tanpa YAML): systemd memuatnya dari /etc/wgpanel.env (0600), dan rahasia tidak perlu
// berpindah ke berkas lain.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
)

// Config adalah konfigurasi yang sudah tervalidasi. Arti tiap field: DESIGN.md §9.1.
type Config struct {
	Environment    string // "development" atau "production"
	Listen         string // alamat HTTP, mis. 127.0.0.1:8091
	DBPath         string
	SecretKey      string // base64 32 byte; divalidasi ulang oleh secret.NewBox
	WGConfDir      string // "" = jangan tulis salinan wg-quick
	TrustedProxies []netip.Prefix
	AllowedOrigins []string
	BackupDir      string
	TelegramToken  string
	MetricsToken   string
}

// IsProduction dipakai untuk keputusan keamanan (cookie Secure, format log).
func (config Config) IsProduction() bool { return config.Environment == "production" }

// Load membaca env dan mengembalikan galat yang menyebut nama variabel bermasalah,
// supaya `journalctl -u wgpanel` langsung menunjukkan apa yang harus diperbaiki.
func Load() (Config, error) {
	config := Config{
		Environment:   envOr("WGPANEL_ENV", "development"),
		Listen:        envOr("WGPANEL_LISTEN", "127.0.0.1:8091"),
		DBPath:        envOr("WGPANEL_DB_PATH", "/var/lib/wgpanel/wgpanel.db"),
		SecretKey:     os.Getenv("WGPANEL_SECRET_KEY"),
		WGConfDir:     envOr("WGPANEL_WG_CONF_DIR", "/etc/wireguard"),
		BackupDir:     envOr("WGPANEL_BACKUP_DIR", "/var/lib/wgpanel/backups"),
		TelegramToken: os.Getenv("WGPANEL_TELEGRAM_BOT_TOKEN"),
		MetricsToken:  os.Getenv("WGPANEL_METRICS_TOKEN"),
	}
	// WGPANEL_WG_CONF_DIR boleh sengaja dikosongkan ("WGPANEL_WG_CONF_DIR=") untuk mematikan salinan.
	if value, set := os.LookupEnv("WGPANEL_WG_CONF_DIR"); set {
		config.WGConfDir = value
	}
	if config.Environment != "development" && config.Environment != "production" {
		return Config{}, fmt.Errorf("WGPANEL_ENV harus development atau production, bukan %q", config.Environment)
	}
	if config.SecretKey == "" {
		return Config{}, errors.New("WGPANEL_SECRET_KEY wajib diisi (buat dengan: openssl rand -base64 32)")
	}
	for _, item := range splitList(os.Getenv("WGPANEL_TRUSTED_PROXIES")) {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return Config{}, fmt.Errorf("WGPANEL_TRUSTED_PROXIES: %q bukan CIDR: %w", item, err)
		}
		config.TrustedProxies = append(config.TrustedProxies, prefix.Masked())
	}
	config.AllowedOrigins = splitList(os.Getenv("WGPANEL_ALLOWED_ORIGINS"))
	return config, nil
}

// envOr mengembalikan nilai env name, atau fallback bila kosong/tidak ada.
func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// splitList memecah "a, b ,c" menjadi ["a","b","c"] dan membuang item kosong.
func splitList(raw string) []string {
	var result []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
