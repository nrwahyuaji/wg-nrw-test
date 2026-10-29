// Package auth: berkas ini mengelola sesi login server-side (DESIGN 7.1): dibuat saat login, dicari di setiap
// request, dicabut saat logout/ganti password, dan dibersihkan berkala oleh sweeper.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"wgpanel/internal/sqlc"
)

// SessionCookieName adalah nama cookie yang membawa token sesi.
const SessionCookieName = "wgp_session"

// SessionIdleTimeout: sesi yang tidak dipakai selama ini dianggap habis.
const SessionIdleTimeout = 30 * time.Minute

// SessionMaxAge: batas absolut sesi sejak login, walau terus dipakai.
const SessionMaxAge = 12 * time.Hour

// Tahap sesi. Sesi selain full hanya boleh memanggil endpoint 2FA dan logout (Fase 14).
const (
	StageFull        = "full"
	StageTOTPPending = "totp_pending"
	StageTOTPEnroll  = "totp_enroll"
)

// ErrNoSession berarti token tidak dikenal, kedaluwarsa, idle terlalu lama, atau milik user nonaktif.
// Sengaja satu galat untuk semua kasus: klien cukup tahu "login lagi".
var ErrNoSession = errors.New("sesi tidak ada atau sudah berakhir")

// SessionStore membungkus query sesi. now dapat diganti di uji untuk mensimulasikan waktu berlalu.
type SessionStore struct {
	queries *sqlc.Queries
	now     func() time.Time
}

// NewSessionStore membuat store dengan jam sistem.
func NewSessionStore(queries *sqlc.Queries) *SessionStore {
	return &SessionStore{queries: queries, now: time.Now}
}

// Create membuat sesi baru untuk userID dan mengembalikan token asli (untuk cookie).
// DB hanya menyimpan hash-nya.
func (store *SessionStore) Create(ctx context.Context, userID int64, stage, ip, userAgent string) (string, error) {
	plain, hash, err := NewToken()
	if err != nil {
		return "", err
	}
	now := store.now().Unix()
	err = store.queries.CreateSession(ctx, sqlc.CreateSessionParams{
		TokenHash: hash, UserID: userID, Stage: stage, Ip: ip, UserAgent: userAgent,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now + int64(SessionMaxAge/time.Second),
	})
	if err != nil {
		return "", err
	}
	return plain, nil
}

// Lookup mencari sesi dari token asli. Menolak (ErrNoSession) sesi yang lewat batas absolut, idle lebih dari
// 30 menit, atau milik user nonaktif/terhapus. Sesi valid diperbarui last_seen_at-nya (paling sering
// sekali per menit, diatur di query). Galat DB lain dikembalikan apa adanya.
func (store *SessionStore) Lookup(ctx context.Context, plain string) (sqlc.Session, sqlc.User, error) {
	if plain == "" {
		return sqlc.Session{}, sqlc.User{}, ErrNoSession
	}
	session, err := store.queries.GetSessionByHash(ctx, HashToken(plain))
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.Session{}, sqlc.User{}, ErrNoSession
	}
	if err != nil {
		return sqlc.Session{}, sqlc.User{}, err
	}
	now := store.now().Unix()
	if now >= session.ExpiresAt || now-session.LastSeenAt > int64(SessionIdleTimeout/time.Second) {
		return sqlc.Session{}, sqlc.User{}, ErrNoSession
	}
	user, err := store.queries.GetUserByID(ctx, session.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.Session{}, sqlc.User{}, ErrNoSession
	}
	if err != nil {
		return sqlc.Session{}, sqlc.User{}, err
	}
	if user.Disabled != 0 {
		return sqlc.Session{}, sqlc.User{}, ErrNoSession
	}
	if err := store.queries.TouchSession(ctx, sqlc.TouchSessionParams{Now: now, TokenHash: session.TokenHash}); err != nil {
		return sqlc.Session{}, sqlc.User{}, err
	}
	return session, user, nil
}

// Delete mencabut satu sesi (logout). Token yang tidak dikenal bukan galat.
func (store *SessionStore) Delete(ctx context.Context, plain string) error {
	return store.queries.DeleteSession(ctx, HashToken(plain))
}

// DeleteOthers mencabut semua sesi userID kecuali sesi dengan hash keepHash (dipakai saat ganti password).
func (store *SessionStore) DeleteOthers(ctx context.Context, userID int64, keepHash string) error {
	return store.queries.DeleteUserSessions(ctx, sqlc.DeleteUserSessionsParams{UserID: userID, ExceptHash: keepHash})
}

// SweepExpired menghapus sesi yang lewat batas absolut atau idle, dan mengembalikan jumlahnya.
func (store *SessionStore) SweepExpired(ctx context.Context) (int64, error) {
	now := store.now()
	return store.queries.DeleteExpiredSessions(ctx, sqlc.DeleteExpiredSessionsParams{
		Now: now.Unix(), IdleBefore: now.Add(-SessionIdleTimeout).Unix(),
	})
}

// RunSweeper menjalankan SweepExpired setiap interval sampai ctx selesai. Galat hanya dicatat: sesi basi
// tetap ditolak Lookup, jadi sweeper yang gagal tidak membuka celah, hanya menunda pembersihan.
func (store *SessionStore) RunSweeper(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed, err := store.SweepExpired(ctx)
			if err != nil {
				slog.Warn("gagal membersihkan sesi kedaluwarsa", "err", err)
				continue
			}
			if removed > 0 {
				slog.Info("sesi kedaluwarsa dibersihkan", "jumlah", removed)
			}
		}
	}
}

// SessionCookie membuat cookie sesi. Tanpa Max-Age/Expires: cookie hilang saat peramban ditutup, dan batas
// sebenarnya ditegakkan server. secure = true di produksi (HTTPS).
func SessionCookie(plain string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name: SessionCookieName, Value: plain, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: secure,
	}
}

// ClearSessionCookie membuat cookie pengganti yang menghapus cookie sesi di peramban (logout).
func ClearSessionCookie(secure bool) *http.Cookie {
	cookie := SessionCookie("", secure)
	cookie.MaxAge = -1
	return cookie
}
