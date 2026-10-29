package config

import "testing"

// TestLoad: kunci wajib, nilai bawaan, daftar proxy, dan penolakan nilai salah.
func TestLoad(t *testing.T) {
	t.Setenv("WGPANEL_SECRET_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("tanpa WGPANEL_SECRET_KEY harus gagal")
	}
	t.Setenv("WGPANEL_SECRET_KEY", "x")
	t.Setenv("WGPANEL_TRUSTED_PROXIES", "10.0.0.5/32, 172.16.0.0/12")
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.Listen != "127.0.0.1:8091" || len(config.TrustedProxies) != 2 || config.IsProduction() {
		t.Fatalf("bawaan salah: %+v", config)
	}
	t.Setenv("WGPANEL_TRUSTED_PROXIES", "bukan-cidr")
	if _, err := Load(); err == nil {
		t.Fatal("proxy bukan CIDR harus gagal")
	}
	t.Setenv("WGPANEL_TRUSTED_PROXIES", "")
	t.Setenv("WGPANEL_ENV", "staging")
	if _, err := Load(); err == nil {
		t.Fatal("WGPANEL_ENV tidak dikenal harus gagal")
	}
}
