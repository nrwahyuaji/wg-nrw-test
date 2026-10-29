// Package scheduler menjalankan penjadwal wgpanel (DESIGN §6.3): kedaluwarsa, kuota, reset bulanan,
// dan backup harian. Satu putaran sinkron saat start, lalu ticker 60 dtk.
package scheduler

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"wgpanel/internal/reconcile"
	"wgpanel/internal/sqlc"
)

// Reconciler minimal yang dibutuhkan scheduler: menerapkan interface ke kernel.
type Reconciler interface {
	WithApply(ctx context.Context, id int64, fn func(tx reconcile.Tx) (int64, error)) error
}

// Scheduler menjalankan satu putaran sinkron saat start, lalu ticker 60 detik.
type Scheduler struct {
	db      *sql.DB
	queries *sqlc.Queries
	recon   Reconciler
	now     func() time.Time
	notify  func(peerID int64, reason string) // no-op sampai Fase 16
}

// New membuat Scheduler. now default time.Now; notify default no-op.
func New(database *sql.DB, recon Reconciler) *Scheduler {
	return &Scheduler{
		db:      database,
		queries: sqlc.New(database),
		recon:   recon,
		now:     time.Now,
		notify:  func(int64, string) {},
	}
}

// Run menjalankan scheduler sampai ctx selesai.
func (s *Scheduler) Run(ctx context.Context) {
	s.tick(ctx)
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick satu putaran: kedaluwarsa, kuota, reset bulanan, apply interface terdampak.
func (s *Scheduler) tick(ctx context.Context) {
	now := s.now().Unix()
	nowTime := s.now()
	// 1. disable expired
	if err := s.queries.DisableExpiredPeers(ctx, sqlc.DisableExpiredPeersParams{UpdatedAt: now, ExpiresAt: sql.NullInt64{Int64: now, Valid: true}}); err != nil {
		slog.Warn("scheduler: disable expired", "err", err)
	}
	// 2. disable over quota
	if err := s.queries.DisableOverQuotaPeers(ctx, now); err != nil {
		slog.Warn("scheduler: disable over quota", "err", err)
	}
	// 3. reset bulanan
	month := nowTime.UTC().Add(8 * time.Hour).Format("2006-01")
	last, _ := s.queries.GetSetting(ctx, "last_monthly_reset")
	if last != month {
		if err := s.queries.ResetMonthlyQuota(ctx, now); err != nil {
			slog.Warn("scheduler: reset quota", "err", err)
		} else {
			if err := s.queries.ReactivateQuotaPeers(ctx, now); err != nil {
				slog.Warn("scheduler: reactivate quota peers", "err", err)
			}
			_ = s.queries.UpsertSetting(ctx, sqlc.UpsertSettingParams{Key: "last_monthly_reset", Value: month})
		}
	}
	// 4. apply interface terdampak
	expiredIfaces, _ := s.queries.ListInterfacesByExpiredPeers(ctx, sql.NullInt64{Int64: now, Valid: true})
	overIfaces, _ := s.queries.ListInterfacesByOverQuotaPeers(ctx)
	seen := map[int64]bool{}
	for _, id := range append(expiredIfaces, overIfaces...) {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := s.recon.WithApply(ctx, id, func(tx reconcile.Tx) (int64, error) { return id, nil }); err != nil {
			slog.Warn("scheduler: apply", "interface_id", id, "err", err)
		}
	}
}
