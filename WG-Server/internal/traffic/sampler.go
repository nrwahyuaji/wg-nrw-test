// Berkas ini berisi sampler trafik (DESIGN §6.1) dan retensi riwayat (DESIGN §6.2) yang dijalankan `serve`.
package traffic

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"wgpanel/internal/sqlc"
	"wgpanel/internal/wg"
)

const (
	// sampleInterval: satu putaran per menit cukup untuk grafik per jam dan kuota, tanpa membebani SQLite.
	sampleInterval = 60 * time.Second
	// pruneInterval: retensi harian berbasis jam, jadi menghapus tiap jam sudah tepat waktu.
	pruneInterval = time.Hour
	// defaultRetentionDays dipakai bila setting traffic_retention_days hilang atau tidak valid (DESIGN §6.2).
	defaultRetentionDays = 90
	secondsPerHour       = 3600
	secondsPerDay        = 86400
)

// StatusReader adalah pembacaan kernel yang dipakai sampler. Produksi = *wg.Engine; uji = palsu.
type StatusReader interface {
	Status(name string) ([]wg.PeerStatus, error)
}

// Sampler mengubah penghitung kumulatif kernel menjadi baris traffic_hourly dan menambah quota_used_bytes.
type Sampler struct {
	db     *sql.DB
	reader StatusReader
	now    func() time.Time // diganti uji agar jam deterministik
}

// NewSampler membuat sampler di atas DB yang sudah bermigrasi dan pembaca kernel.
func NewSampler(database *sql.DB, reader StatusReader) *Sampler {
	return &Sampler{db: database, reader: reader, now: time.Now}
}

// Run menjalankan satu putaran sampel + retensi saat start, lalu sampel tiap 60 dtk dan retensi tiap jam,
// sampai ctx selesai. Galat satu putaran hanya dicatat: sampler tidak boleh berhenti karena satu kegagalan
// sementara (mis. SQLITE_BUSY atau interface sedang dibuat ulang).
func (sampler *Sampler) Run(ctx context.Context) {
	sampleTicker := time.NewTicker(sampleInterval)
	defer sampleTicker.Stop()
	pruneTicker := time.NewTicker(pruneInterval)
	defer pruneTicker.Stop()
	sampler.sampleAndLog(ctx)
	sampler.pruneAndLog(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-sampleTicker.C:
			sampler.sampleAndLog(ctx)
		case <-pruneTicker.C:
			sampler.pruneAndLog(ctx)
		}
	}
}

// sampleAndLog menjalankan SampleOnce dan mencatat galatnya.
func (sampler *Sampler) sampleAndLog(ctx context.Context) {
	if err := sampler.SampleOnce(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("putaran sampler trafik gagal", "err", err)
	}
}

// pruneAndLog menjalankan Prune dan mencatat hasilnya.
func (sampler *Sampler) pruneAndLog(ctx context.Context) {
	removed, err := sampler.Prune(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("retensi trafik gagal", "err", err)
		}
		return
	}
	if removed > 0 {
		slog.Info("riwayat trafik lama dihapus", "baris", removed)
	}
}

// reading adalah satu pembacaan kernel yang sudah dipetakan ke interface DB.
type reading struct {
	interfaceID int64
	counter     Counter
}

// SampleOnce menjalankan satu putaran: Status sekali per interface aktif, lalu SATU transaksi yang menulis
// traffic_hourly, peer_counters, dan quota_used_bytes untuk semua peer (DESIGN §6.1, N-6).
// Peer tanpa peer_counters hanya disimpan sebagai baseline (tanpa delta): penghitung kernel kumulatif sejak
// interface dibuat, jadi menghitungnya sebagai trafik baru akan menggandakan pemakaian setiap panel restart.
// Interface yang gagal dibaca dilewati (peer-nya tidak bertambah putaran ini), bukan menggagalkan putaran.
func (sampler *Sampler) SampleOnce(ctx context.Context) error {
	interfaces, err := sqlc.New(sampler.db).ListInterfaces(ctx)
	if err != nil {
		return err
	}
	// Kernel dibaca sebelum transaksi: pool DB satu koneksi, jadi netlink tidak boleh memegang koneksi itu.
	readings := map[string]reading{}
	for _, iface := range interfaces {
		if iface.Enabled == 0 {
			continue
		}
		statuses, err := sampler.reader.Status(iface.Name)
		if err != nil {
			slog.Debug("sampler: interface dilewati", "interface", iface.Name, "err", err)
			continue
		}
		for _, status := range statuses {
			readings[status.PublicKey] = reading{
				interfaceID: iface.ID,
				counter:     Counter{ReceiveBytes: status.ReceiveBytes, TransmitBytes: status.TransmitBytes},
			}
		}
	}
	if len(readings) == 0 {
		return nil
	}

	moment := sampler.now().Unix()
	hour := moment - moment%secondsPerHour
	tx, err := sampler.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op setelah Commit
	queries := sqlc.New(tx)
	// Peer dan counter dibaca di dalam transaksi supaya peer yang baru dihapus tidak ditulis (FK gagal).
	peers, err := queries.ListPeerKeys(ctx)
	if err != nil {
		return err
	}
	counters, err := queries.ListPeerCounters(ctx)
	if err != nil {
		return err
	}
	previousByPeer := make(map[int64]Counter, len(counters))
	for _, row := range counters {
		previousByPeer[row.PeerID] = Counter{ReceiveBytes: row.RxBytes, TransmitBytes: row.TxBytes}
	}
	for _, peer := range peers {
		current, ok := readings[peer.PublicKey]
		// Interface harus cocok: public key yang sama di interface lain bukan peer ini.
		if !ok || current.interfaceID != peer.InterfaceID {
			continue
		}
		if previous, seen := previousByPeer[peer.ID]; seen {
			delta := Delta(previous, current.counter)
			if total := delta.ReceiveBytes + delta.TransmitBytes; total > 0 {
				if err := queries.UpsertTrafficHour(ctx, sqlc.UpsertTrafficHourParams{
					PeerID: peer.ID, Hour: hour, RxBytes: delta.ReceiveBytes, TxBytes: delta.TransmitBytes,
				}); err != nil {
					return fmt.Errorf("traffic: jam peer %d: %w", peer.ID, err)
				}
				if err := queries.AddQuotaUsed(ctx, sqlc.AddQuotaUsedParams{QuotaUsedBytes: total, ID: peer.ID}); err != nil {
					return fmt.Errorf("traffic: kuota peer %d: %w", peer.ID, err)
				}
			}
		}
		if err := queries.UpsertPeerCounter(ctx, sqlc.UpsertPeerCounterParams{
			PeerID: peer.ID, RxBytes: current.counter.ReceiveBytes, TxBytes: current.counter.TransmitBytes, ReadAt: moment,
		}); err != nil {
			return fmt.Errorf("traffic: counter peer %d: %w", peer.ID, err)
		}
	}
	return tx.Commit()
}

// Prune menghapus traffic_hourly yang lebih tua dari traffic_retention_days (bawaan 90). Return jumlah baris.
func (sampler *Sampler) Prune(ctx context.Context) (int64, error) {
	queries := sqlc.New(sampler.db)
	days := defaultRetentionDays
	if value, err := queries.GetSetting(ctx, "traffic_retention_days"); err == nil {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			days = parsed
		}
	}
	return queries.DeleteTrafficBefore(ctx, sampler.now().Unix()-int64(days)*secondsPerDay)
}
