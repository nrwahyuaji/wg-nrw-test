// Package db membuka basis data SQLite wgpanel dan menjalankan migrasi yang ter-embed di biner.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // driver "sqlite" tanpa CGO, agar biner tetap CGO_ENABLED=0
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open membuka (atau membuat) berkas DB dengan pragma yang dibutuhkan wgpanel:
// WAL agar pembaca tidak menunggu penulis, busy_timeout agar penulisan bersamaan menunggu alih-alih galat,
// dan foreign_keys karena SQLite mematikannya secara bawaan per koneksi.
// Pool sengaja satu koneksi: SQLite hanya menerima satu penulis, dan sampler + request yang menulis
// bersamaan akan saling memicu SQLITE_BUSY bila koneksinya banyak.
func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		_ = database.Close() // galat Ping yang dikembalikan sudah menjelaskan penyebabnya
		return nil, fmt.Errorf("db: membuka %s: %w", path, err)
	}
	return database, nil
}

// Migrate menjalankan semua migrasi yang belum diterapkan. Return jumlah migrasi yang baru dijalankan,
// sehingga start kedua bisa mencatat "0 migrasi" sebagai bukti idempoten.
func Migrate(ctx context.Context, database *sql.DB) (int, error) {
	provider, err := goose.NewProvider(goose.DialectSQLite3, database, mustSub())
	if err != nil {
		return 0, err
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return 0, fmt.Errorf("db: migrasi: %w", err)
	}
	return len(results), nil
}
