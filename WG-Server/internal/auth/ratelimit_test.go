package auth

import (
	"fmt"
	"testing"
	"time"
)

// newTestLimiter membuat LoginLimiter dengan jam yang bisa digeser.
func newTestLimiter() (*LoginLimiter, *time.Time) {
	clock := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	limiter := NewLoginLimiter()
	limiter.now = func() time.Time { return clock }
	return limiter, &clock
}

// TestLoginLimitPerIP: 5 percobaan dalam semenit lolos, ke-6 ditolak dengan Retry-After, IP lain tidak
// terpengaruh, dan jatah pulih setelah waktu berlalu.
func TestLoginLimitPerIP(t *testing.T) {
	limiter, clock := newTestLimiter()
	for attempt := 1; attempt <= 5; attempt++ {
		if allowed, _ := limiter.Allow("10.0.0.1", fmt.Sprintf("user%d", attempt)); !allowed {
			t.Fatalf("percobaan ke-%d harus lolos", attempt)
		}
	}
	allowed, retryAfter := limiter.Allow("10.0.0.1", "lain")
	if allowed || retryAfter <= 0 || retryAfter > 12*time.Second {
		t.Fatalf("ke-6 harus ditolak dengan Retry-After 0–12 dtk: %v %v", allowed, retryAfter)
	}
	if allowed, _ := limiter.Allow("10.0.0.2", "lain"); !allowed {
		t.Fatal("IP lain tidak boleh terkena batas IP pertama")
	}
	*clock = clock.Add(time.Minute)
	if allowed, _ := limiter.Allow("10.0.0.1", "lain"); !allowed {
		t.Fatal("setelah semenit jatah IP harus pulih")
	}
}

// TestLoginLimitUsernameLockout: 10 gagal dalam 15 menit (dari banyak IP) mengunci username 15 menit tanpa
// peduli huruf besar/kecil; sukses menghapus hitungan; kunci lepas setelah 15 menit.
func TestLoginLimitUsernameLockout(t *testing.T) {
	limiter, clock := newTestLimiter()
	for attempt := 1; attempt <= 9; attempt++ {
		limiter.RecordFailure("Admin")
	}
	if allowed, _ := limiter.Allow("10.0.0.50", "admin"); !allowed {
		t.Fatal("9 gagal belum boleh mengunci")
	}
	limiter.RecordFailure("ADMIN")
	allowed, retryAfter := limiter.Allow("10.0.0.51", "admin")
	if allowed || retryAfter != usernameLockout {
		t.Fatalf("gagal ke-10 harus mengunci 15 menit: %v %v", allowed, retryAfter)
	}
	if allowed, _ := limiter.Allow("10.0.0.52", "operator"); !allowed {
		t.Fatal("username lain tidak boleh ikut terkunci")
	}
	*clock = clock.Add(usernameLockout)
	if allowed, _ := limiter.Allow("10.0.0.53", "admin"); !allowed {
		t.Fatal("kunci harus lepas setelah 15 menit")
	}

	// Gagal yang tersebar melewati jendela 15 menit tidak menumpuk; sukses mereset hitungan.
	for attempt := 1; attempt <= 9; attempt++ {
		limiter.RecordFailure("budi")
	}
	*clock = clock.Add(usernameFailWindow + time.Second)
	limiter.RecordFailure("budi")
	if allowed, _ := limiter.Allow("10.0.0.54", "budi"); !allowed {
		t.Fatal("gagal di luar jendela 15 menit tidak boleh dihitung bersama")
	}
	for attempt := 1; attempt <= 9; attempt++ {
		limiter.RecordFailure("citra")
	}
	limiter.RecordSuccess("citra")
	limiter.RecordFailure("citra")
	if allowed, _ := limiter.Allow("10.0.0.55", "citra"); !allowed {
		t.Fatal("sukses harus mereset hitungan gagal")
	}
}

// TestLoginLimitCleanup: limiter IP menganggur dan catatan username basi dibuang, yang masih terkunci tetap ada.
func TestLoginLimitCleanup(t *testing.T) {
	limiter, clock := newTestLimiter()
	limiter.Allow("10.0.0.1", "a")
	limiter.RecordFailure("basi")
	*clock = clock.Add(usernameFailWindow + time.Second)
	for attempt := 1; attempt <= 10; attempt++ {
		limiter.RecordFailure("terkunci")
	}
	limiter.Cleanup()
	if len(limiter.perIP) != 0 {
		t.Fatalf("limiter IP menganggur harus dibuang: %d", len(limiter.perIP))
	}
	if _, kept := limiter.usernames["basi"]; kept {
		t.Fatal("catatan username basi harus dibuang")
	}
	if _, kept := limiter.usernames["terkunci"]; !kept {
		t.Fatal("username yang masih terkunci tidak boleh dibuang")
	}
}
