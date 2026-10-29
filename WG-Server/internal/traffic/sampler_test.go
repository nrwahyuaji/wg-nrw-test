package traffic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"wgpanel/internal/db"
	"wgpanel/internal/wg"
)

// fakeReader mengembalikan rx=value, tx=value/2 untuk semua public key di keys.
type fakeReader struct {
	keys  []string
	value int64
	fail  bool
}

func (reader *fakeReader) Status(string) ([]wg.PeerStatus, error) {
	if reader.fail {
		return nil, errors.New("kernel tidak terbaca")
	}
	result := make([]wg.PeerStatus, 0, len(reader.keys))
	for _, key := range reader.keys {
		result = append(result, wg.PeerStatus{PublicKey: key, ReceiveBytes: reader.value, TransmitBytes: reader.value / 2})
	}
	return result, nil
}

// fixture membuat DB bermigrasi berisi wg0 dan count peer (public key "kunci-N"). Return DB + daftar key.
func fixture(tb testing.TB, count int) (*sql.DB, []string) {
	tb.Helper()
	database, err := db.Open(filepath.Join(tb.TempDir(), "uji.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = database.Close() })
	if _, err := db.Migrate(context.Background(), database); err != nil {
		tb.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO interfaces (id, name, address, listen_port, private_key, public_key,
		endpoint_host, created_at, updated_at) VALUES (1, 'wg0', '10.8.0.1/22', 51820, x'00', 'srv', 'vpn', 1, 1)`); err != nil {
		tb.Fatal(err)
	}
	keys := make([]string, 0, count)
	for index := range count {
		key := fmt.Sprintf("kunci-%d", index)
		if _, err := database.Exec(`INSERT INTO peers (interface_id, name, address, public_key, created_at, updated_at)
			VALUES (1, ?, ?, ?, 1, 1)`, key, fmt.Sprintf("10.8.%d.%d/32", (index+2)/256, (index+2)%256), key); err != nil {
			tb.Fatal(err)
		}
		keys = append(keys, key)
	}
	return database, keys
}

// TestSamplerDeltaAndReset (PLAN 8.1): rx 100 → 160 → 50 (reset). Putaran 1 hanya baseline, putaran 2 +60,
// putaran 3 +50, sehingga rx jam berjalan = 110. tx = rx/2 (50 → 80 → 25) = 55; quota_used_bytes = rx+tx = 165.
func TestSamplerDeltaAndReset(t *testing.T) {
	database, keys := fixture(t, 1)
	reader := &fakeReader{keys: keys}
	sampler := NewSampler(database, reader)
	fixed := time.Unix(1_800_001_234, 0)
	sampler.now = func() time.Time { return fixed }
	ctx := context.Background()

	for _, value := range []int64{100, 160, 50} {
		reader.value = value
		if err := sampler.SampleOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var hour, rx, tx, used int64
	if err := database.QueryRow(`SELECT hour, rx_bytes, tx_bytes FROM traffic_hourly`).Scan(&hour, &rx, &tx); err != nil {
		t.Fatal(err)
	}
	if hour != fixed.Unix()-fixed.Unix()%3600 || rx != 110 || tx != 55 {
		t.Fatalf("traffic_hourly: hour=%d rx=%d tx=%d", hour, rx, tx)
	}
	if err := database.QueryRow(`SELECT quota_used_bytes FROM peers`).Scan(&used); err != nil || used != 165 {
		t.Fatalf("quota_used_bytes = %d (%v), mau 165", used, err)
	}
	var counter int64
	if err := database.QueryRow(`SELECT rx_bytes FROM peer_counters`).Scan(&counter); err != nil || counter != 50 {
		t.Fatalf("peer_counters harus pembacaan terakhir 50: %d %v", counter, err)
	}

	// Kernel gagal dibaca: putaran tidak galat dan tidak mengubah apa pun.
	reader.fail = true
	if err := sampler.SampleOnce(ctx); err != nil {
		t.Fatalf("interface gagal dibaca harus dilewati: %v", err)
	}
}

// TestSamplerRetention: baris lebih tua dari traffic_retention_days terhapus, yang lebih baru tetap.
func TestSamplerRetention(t *testing.T) {
	database, _ := fixture(t, 1)
	sampler := NewSampler(database, &fakeReader{})
	fixed := time.Unix(1_800_000_000, 0)
	sampler.now = func() time.Time { return fixed }
	if _, err := database.Exec(`INSERT INTO settings (key, value) VALUES ('traffic_retention_days', '2')`); err != nil {
		t.Fatal(err)
	}
	for _, age := range []int64{3 * 86400, 86400} {
		if _, err := database.Exec(`INSERT INTO traffic_hourly (peer_id, hour, rx_bytes, tx_bytes) VALUES (1, ?, 1, 1)`,
			fixed.Unix()-age); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := sampler.Prune(context.Background())
	if err != nil || removed != 1 {
		t.Fatalf("Prune: %d %v, mau 1", removed, err)
	}
}

// BenchmarkSampler: satu putaran 500 peer (target N-6 < 50 ms), setelah baseline terisi.
func BenchmarkSampler(b *testing.B) {
	database, keys := fixture(b, 500)
	reader := &fakeReader{keys: keys, value: 1}
	sampler := NewSampler(database, reader)
	ctx := context.Background()
	if err := sampler.SampleOnce(ctx); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		reader.value++
		if err := sampler.SampleOnce(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
