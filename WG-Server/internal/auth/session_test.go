package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"wgpanel/internal/db"
	"wgpanel/internal/sqlc"
)

// newTestStore membuat DB sementara bermigrasi, satu user, dan SessionStore dengan jam yang bisa digeser.
func newTestStore(t *testing.T) (*SessionStore, *sql.DB, sqlc.User, *time.Time) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "uji.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := db.Migrate(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	queries := sqlc.New(database)
	user, err := queries.CreateUser(context.Background(), sqlc.CreateUserParams{
		Username: "admin", PasswordHash: "h", Role: "admin", CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	store := NewSessionStore(queries)
	store.now = func() time.Time { return clock }
	return store, database, user, &clock
}

// TestSessionLifecycle: dibuat→ditemukan, idle 31 menit ditolak, lewat 12 jam ditolak walau aktif,
// user disabled ditolak, token acak → ErrNoSession, dan sweeper menghapus sesi basi.
func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()

	t.Run("dibuat lalu ditemukan", func(t *testing.T) {
		store, _, user, _ := newTestStore(t)
		token, err := store.Create(ctx, user.ID, StageFull, "10.0.0.1", "uji")
		if err != nil {
			t.Fatal(err)
		}
		session, found, err := store.Lookup(ctx, token)
		if err != nil || found.ID != user.ID || session.Stage != StageFull {
			t.Fatalf("Lookup: %+v %+v %v", session, found, err)
		}
		if session.TokenHash == token {
			t.Fatal("DB tidak boleh menyimpan token asli")
		}
	})

	t.Run("idle 31 menit ditolak, 29 menit diterima", func(t *testing.T) {
		store, _, user, clock := newTestStore(t)
		token, _ := store.Create(ctx, user.ID, StageFull, "ip", "ua")
		*clock = clock.Add(29 * time.Minute)
		if _, _, err := store.Lookup(ctx, token); err != nil {
			t.Fatalf("29 menit harus diterima: %v", err)
		}
		*clock = clock.Add(31 * time.Minute) // 31 menit sejak sentuhan terakhir
		if _, _, err := store.Lookup(ctx, token); !errors.Is(err, ErrNoSession) {
			t.Fatalf("idle 31 menit harus ErrNoSession, got %v", err)
		}
	})

	t.Run("lewat 12 jam ditolak walau terus aktif", func(t *testing.T) {
		store, _, user, clock := newTestStore(t)
		token, _ := store.Create(ctx, user.ID, StageFull, "ip", "ua")
		// Disentuh setiap 20 menit (tidak pernah idle) selama 11 jam 40 menit: harus selalu diterima.
		for range 35 {
			*clock = clock.Add(20 * time.Minute)
			if _, _, err := store.Lookup(ctx, token); err != nil {
				t.Fatalf("masih dalam 12 jam harus diterima (%s): %v", clock.Format(time.TimeOnly), err)
			}
		}
		*clock = clock.Add(20 * time.Minute) // tepat 12 jam sejak login
		if _, _, err := store.Lookup(ctx, token); !errors.Is(err, ErrNoSession) {
			t.Fatalf("lewat 12 jam harus ErrNoSession, got %v", err)
		}
	})

	t.Run("user disabled ditolak", func(t *testing.T) {
		store, database, user, _ := newTestStore(t)
		token, _ := store.Create(ctx, user.ID, StageFull, "ip", "ua")
		// Query menonaktifkan user baru ada di Fase 13; uji memakai SQL langsung.
		if _, err := database.Exec(`UPDATE users SET disabled = 1 WHERE id = ?`, user.ID); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Lookup(ctx, token); !errors.Is(err, ErrNoSession) {
			t.Fatalf("user disabled harus ErrNoSession, got %v", err)
		}
	})

	t.Run("token acak lain ErrNoSession", func(t *testing.T) {
		store, _, user, _ := newTestStore(t)
		if _, err := store.Create(ctx, user.ID, StageFull, "ip", "ua"); err != nil {
			t.Fatal(err)
		}
		other, _, _ := NewToken()
		for _, token := range []string{other, ""} {
			if _, _, err := store.Lookup(ctx, token); !errors.Is(err, ErrNoSession) {
				t.Fatalf("token %q harus ErrNoSession, got %v", token, err)
			}
		}
	})

	t.Run("logout, cabut sesi lain, sweeper", func(t *testing.T) {
		store, _, user, clock := newTestStore(t)
		first, _ := store.Create(ctx, user.ID, StageFull, "ip", "ua")
		second, _ := store.Create(ctx, user.ID, StageFull, "ip", "ua")
		third, _ := store.Create(ctx, user.ID, StageFull, "ip", "ua")
		if err := store.Delete(ctx, first); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Lookup(ctx, first); !errors.Is(err, ErrNoSession) {
			t.Fatalf("sesi logout harus hilang, got %v", err)
		}
		if err := store.DeleteOthers(ctx, user.ID, HashToken(second)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.Lookup(ctx, third); !errors.Is(err, ErrNoSession) {
			t.Fatalf("sesi lain harus dicabut, got %v", err)
		}
		if _, _, err := store.Lookup(ctx, second); err != nil {
			t.Fatalf("sesi yang dipertahankan harus tetap ada: %v", err)
		}
		*clock = clock.Add(SessionIdleTimeout + time.Minute)
		if removed, err := store.SweepExpired(ctx); err != nil || removed != 1 {
			t.Fatalf("sweeper harus menghapus 1 sesi idle: %d %v", removed, err)
		}
	})
}
