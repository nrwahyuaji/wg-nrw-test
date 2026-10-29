// Package auth: berkas ini membatasi laju percobaan login per IP dan per username (DESIGN 7.3).
package auth

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Batas login. Per IP: 5 percobaan per menit (burst 5), menahan tebakan cepat dari satu sumber.
// Per username: 10 gagal dalam 15 menit lalu dikunci 15 menit, menahan tebakan tersebar dari banyak IP.
const (
	loginPerIPPerMinute = 5
	usernameMaxFailures = 10
	usernameFailWindow  = 15 * time.Minute
	usernameLockout     = 15 * time.Minute
	limiterCleanupEvery = 10 * time.Minute
)

// failState mencatat kegagalan login satu username dalam jendela berjalan.
type failState struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
}

// ipEntry menyimpan limiter per IP beserta waktu terakhir dipakai, untuk dibersihkan bila menganggur.
type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// LoginLimiter aman dipakai bersamaan. Keadaan hanya di memori: restart mengosongkan hitungan, yang dapat
// diterima karena restart butuh akses root dan bcrypt cost 12 tetap memperlambat tebakan.
type LoginLimiter struct {
	mutex     sync.Mutex
	perIP     map[string]*ipEntry
	usernames map[string]*failState
	now       func() time.Time
}

// NewLoginLimiter membuat limiter dengan jam sistem.
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{perIP: map[string]*ipEntry{}, usernames: map[string]*failState{}, now: time.Now}
}

// Allow dipanggil sekali per percobaan login, sebelum password diperiksa. Mengurangi jatah IP, lalu menolak bila
// jatah IP habis atau username sedang dikunci. Bila ditolak, retryAfter > 0 berisi saran untuk header Retry-After.
func (limiter *LoginLimiter) Allow(ip, username string) (allowed bool, retryAfter time.Duration) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.now()
	entry := limiter.perIP[ip]
	if entry == nil {
		entry = &ipEntry{limiter: rate.NewLimiter(rate.Every(time.Minute/loginPerIPPerMinute), loginPerIPPerMinute)}
		limiter.perIP[ip] = entry
	}
	entry.lastSeen = now
	reservation := entry.limiter.ReserveN(now, 1)
	if delay := reservation.DelayFrom(now); delay > 0 {
		// Jatah dikembalikan: percobaan yang ditolak tidak boleh memperpanjang hukuman sendiri.
		reservation.CancelAt(now)
		return false, delay
	}
	if state := limiter.usernames[normalizeUsername(username)]; state != nil && now.Before(state.lockedUntil) {
		return false, state.lockedUntil.Sub(now)
	}
	return true, 0
}

// RecordFailure mencatat password salah untuk username (juga username yang tidak ada, agar penyerang tidak bisa
// membedakannya dari perilaku kunci). Kegagalan ke-10 dalam 15 menit mengunci username 15 menit.
func (limiter *LoginLimiter) RecordFailure(username string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.now()
	key := normalizeUsername(username)
	state := limiter.usernames[key]
	if state == nil || now.Sub(state.windowStart) > usernameFailWindow {
		state = &failState{windowStart: now}
		limiter.usernames[key] = state
	}
	state.failures++
	if state.failures >= usernameMaxFailures {
		state.lockedUntil = now.Add(usernameLockout)
		state.failures = 0
		state.windowStart = now
	}
}

// RecordSuccess menghapus catatan gagal username setelah login benar.
func (limiter *LoginLimiter) RecordSuccess(username string) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	delete(limiter.usernames, normalizeUsername(username))
}

// Cleanup membuang limiter IP yang menganggur dan catatan username yang jendela serta kuncinya sudah lewat,
// supaya map tidak tumbuh tanpa batas oleh IP/username acak.
func (limiter *LoginLimiter) Cleanup() {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.now()
	for ip, entry := range limiter.perIP {
		// Setelah 1 menit menganggur jatah sudah pulih penuh, jadi limiter baru setara dengan yang lama.
		if now.Sub(entry.lastSeen) > time.Minute {
			delete(limiter.perIP, ip)
		}
	}
	for name, state := range limiter.usernames {
		if now.Sub(state.windowStart) > usernameFailWindow && !now.Before(state.lockedUntil) {
			delete(limiter.usernames, name)
		}
	}
}

// RunCleanup memanggil Cleanup setiap 10 menit sampai ctx selesai.
func (limiter *LoginLimiter) RunCleanup(ctx context.Context) {
	ticker := time.NewTicker(limiterCleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			limiter.Cleanup()
		}
	}
}

// normalizeUsername menyamakan huruf besar/kecil seperti kolom username (COLLATE NOCASE), agar "Admin" dan
// "admin" berbagi satu hitungan gagal.
func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
