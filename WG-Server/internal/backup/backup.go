// Package backup membuat snapshot basis data SQLite (PLAN 11.1, PRD F-16.1).
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxSnapshots = 14

// Snapshot membuat salinan DB di dir dengan nama wgpanel-<waktu>.db, memverifikasi integrity,
// lalu merotasi agar hanya maxSnapshots salinan terbaru yang tersisa. Mengembalikan path salinan.
func Snapshot(ctx context.Context, db *sql.DB, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("backup: buat dir: %w", err)
	}
	path := filepath.Join(dir, "wgpanel-"+time.Now().UTC().Format("20060102-150405")+".db")
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("backup: VACUUM INTO: %w", err)
	}
	check, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("backup: buka salinan: %w", err)
	}
	defer func() { _ = check.Close() }()
	var result string
	if err := check.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("backup: integrity_check: %w", err)
	}
	if result != "ok" {
		_ = os.Remove(path)
		return "", fmt.Errorf("backup: integrity_check = %s", result)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
	_ = rotate(dir, maxSnapshots)
	return path, nil
}

func rotate(dir string, n int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var snaps []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "wgpanel-") && strings.HasSuffix(name, ".db") {
			snaps = append(snaps, name)
		}
	}
	if len(snaps) <= n {
		return nil
	}
	sort.Strings(snaps)
	for _, name := range snaps[:len(snaps)-n] {
		_ = os.Remove(filepath.Join(dir, name))
	}
	return nil
}
