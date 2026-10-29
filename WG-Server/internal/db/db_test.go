package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMigrateTwice: migrasi pertama menerapkan semua berkas, kedua tidak menerapkan apa pun,
// dan foreign key benar-benar aktif (peer dengan interface_id fiktif ditolak).
func TestMigrateTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uji.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	ctx := context.Background()
	first, err := Migrate(ctx, database)
	if err != nil || first == 0 {
		t.Fatalf("migrasi pertama: %d %v", first, err)
	}
	second, err := Migrate(ctx, database)
	if err != nil || second != 0 {
		t.Fatalf("migrasi kedua harus 0: %d %v", second, err)
	}
	_, err = database.Exec(`INSERT INTO peers (interface_id, name, address, public_key, created_at, updated_at)
		VALUES (999, 'x', '10.8.0.2/32', 'k', 0, 0)`)
	if err == nil {
		t.Fatal("foreign key harus menolak interface_id fiktif")
	}
	var mode string
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode %s", mode)
	}
}
