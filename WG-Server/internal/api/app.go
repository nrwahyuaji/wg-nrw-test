// Package api memegang router HTTP, middleware, dan handler REST /api/v1 wgpanel.
// Berkas ini mendefinisikan App: semua dependensi yang dipakai handler.
package api

import (
	"database/sql"

	"wgpanel/internal/auth"
	"wgpanel/internal/config"
	"wgpanel/internal/reconcile"
	"wgpanel/internal/secret"
	"wgpanel/internal/sqlc"
)

// App memegang dependensi bersama handler. Dibuat sekali di `serve`. Field Audit menyusul di Fase 10.
type App struct {
	DB       *sql.DB
	Queries  *sqlc.Queries
	Config   config.Config
	Sessions *auth.SessionStore
	Limiter  *auth.LoginLimiter
	Box      *secret.Box // kunci private key WireGuard; dipakai mulai Fase 4
	// Reconciler menerapkan perubahan DB ke kernel (Fase 4). Dipasang oleh `serve` setelah Engine terbuka,
	// karena NewApp dipakai juga oleh uji tanpa akses kernel.
	Reconciler *reconcile.Reconciler
	// Kernel membaca keadaan interface untuk /health. Dipasang `serve` (= Engine yang sama dengan Reconciler).
	Kernel KernelInspector
}

// NewApp merangkai App dari DB yang sudah bermigrasi dan config tervalidasi. Galat bila WGPANEL_SECRET_KEY
// bukan base64 32 byte, sehingga kunci salah ketahuan saat start, bukan saat key pertama dibuka.
func NewApp(database *sql.DB, cfg config.Config) (*App, error) {
	box, err := secret.NewBox(cfg.SecretKey, "wg-keys")
	if err != nil {
		return nil, err
	}
	queries := sqlc.New(database)
	return &App{
		DB:       database,
		Queries:  queries,
		Config:   cfg,
		Sessions: auth.NewSessionStore(queries),
		Limiter:  auth.NewLoginLimiter(),
		Box:      box,
	}, nil
}
