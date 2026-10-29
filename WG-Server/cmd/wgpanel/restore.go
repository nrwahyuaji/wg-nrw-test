// Package main: subperintah restore (PLAN 11.2). Memulihkan DB dari snapshot dengan pemeriksaan kunci.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"wgpanel/internal/config"
	"wgpanel/internal/db"
	"wgpanel/internal/secret"
	"wgpanel/internal/sqlc"
)

func runRestore(args []string) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	file := flags.String("file", "", "berkas snapshot (wajib)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("-file wajib diisi")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if out, _ := exec.Command("systemctl", "is-active", "wgpanel.service").Output(); strings.TrimSpace(string(out)) == "active" {
		return errors.New("wgpanel.service aktif: hentikan dulu sebelum restore")
	}
	snap, err := sql.Open("sqlite", "file:"+*file+"?mode=ro")
	if err != nil {
		return err
	}
	var integrity string
	if err := snap.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		_ = snap.Close()
		return err
	}
	_ = snap.Close()
	if integrity != "ok" {
		return fmt.Errorf("integrity_check snapshot = %s", integrity)
	}
	box, err := secret.NewBox(cfg.SecretKey, "wg-keys")
	if err != nil {
		return err
	}
	dbSnap, err := db.Open(*file)
	if err != nil {
		return err
	}
	queries := sqlc.New(dbSnap)
	value, err := queries.GetSetting(context.Background(), "key_check")
	_ = dbSnap.Close()
	if err == nil && value != "" {
		if _, err := box.Open([]byte(value)); err != nil {
			return errors.New("kunci tidak cocok, restore dibatalkan")
		}
	}
	backupPath := cfg.DBPath + ".pre-restore-" + time.Now().UTC().Format("20060102-150405")
	if err := copyFile(cfg.DBPath, backupPath); err != nil {
		return err
	}
	if err := copyFile(*file, cfg.DBPath); err != nil {
		_ = copyFile(backupPath, cfg.DBPath)
		return err
	}
	fmt.Println("restore selesai, jalankan `wgpanel apply` untuk menerapkan ke kernel")
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
