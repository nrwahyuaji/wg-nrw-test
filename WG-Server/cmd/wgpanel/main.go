// Command wgpanel adalah satu-satunya biner WG-WebUI. Subperintah:
//
//	serve   jalankan panel web + sampler + scheduler (unit wgpanel.service)
//	apply   samakan kernel dengan DB lalu keluar (unit wgpanel-apply.service, saat boot)
//	version cetak versi build
//	create-admin -username <u>  buat akun admin; password dibaca dari terminal
//
// Subperintah lain (import-conf, restore, doctor) ditambahkan di fase masing-masing.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"wgpanel/internal/api"
	"wgpanel/internal/auth"
	"wgpanel/internal/config"
	"wgpanel/internal/db"
	"wgpanel/internal/reconcile"
	"wgpanel/internal/scheduler"
	"wgpanel/internal/secret"
	"wgpanel/internal/sqlc"
	"wgpanel/internal/traffic"
	"wgpanel/internal/wg"
)

// main memilih subperintah. Setiap subperintah memakai FlagSet sendiri (bila perlu flag), karena
// flag.Parse global berhenti di argumen pertama yang bukan flag.
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "pakai: wgpanel serve|apply|version|create-admin")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe()
	case "apply":
		err = runApply()
	case "version":
		fmt.Println(buildVersion())
	case "restore":
		err = runRestore(os.Args[2:])
	case "create-admin":
		err = runCreateAdmin(os.Args[2:])
	default:
		err = fmt.Errorf("subperintah tidak dikenal: %s", os.Args[1])
	}
	if err != nil {
		slog.Error("wgpanel berhenti", "err", err)
		os.Exit(1)
	}
}

// openDatabase memuat config, membuka DB, dan menjalankan migrasi. Dipakai serve dan apply,
// sehingga keduanya selalu melihat skema yang sama.
func openDatabase(ctx context.Context) (config.Config, *sql.DB, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, err
	}
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return config.Config{}, nil, err
	}
	applied, err := db.Migrate(ctx, database)
	if err != nil {
		_ = database.Close() // galat migrasi yang dikembalikan sudah menjelaskan penyebabnya
		return config.Config{}, nil, err
	}
	slog.Info("migrasi selesai", "baru", applied)
	return cfg, database, nil
}

// runServe menyalakan HTTP. Port diikat dengan net.Listen lebih dulu, sehingga port yang sudah dipakai
// menghasilkan pesan jelas, bukan stack trace. SIGTERM memicu shutdown rapi (≤ 10 detik).
func runServe() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg, database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer closeDatabase(database)
	app, err := api.NewApp(database, cfg)
	if err != nil {
		return err
	}
	engine, err := wg.NewEngine()
	if err != nil {
		return err
	}
	defer func() { _ = engine.Close() }()
	app.Reconciler = reconcile.New(database, engine, app.Box, cfg.WGConfDir)
	app.Kernel = engine
	// Kegagalan satu interface tidak menghentikan panel: admin tetap perlu UI untuk memperbaikinya.
	if applied, err := app.Reconciler.ApplyAll(ctx); err != nil {
		slog.Error("apply awal sebagian gagal", "aktif", applied, "err", err)
	} else {
		slog.Info("apply awal selesai", "aktif", applied)
	}

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("gagal mengikat %s — kemungkinan sudah dipakai proses lain: %w", cfg.Listen, err)
	}
	server := &http.Server{Handler: api.NewRouter(app), ReadHeaderTimeout: 10 * time.Second}
	// Sweeper sesi & pembersih limiter berhenti bersama ctx sinyal. Sesi basi tetap ditolak Lookup walau belum tersapu.
	go app.Sessions.RunSweeper(ctx, time.Hour)
	go app.Limiter.RunCleanup(ctx)
	// Scheduler (Fase 9): kedaluwarsa, kuota, reset bulanan.
	go scheduler.New(database, app.Reconciler).Run(ctx)
	// Sampler trafik (DESIGN §6.1): putaran pertama langsung saat start, lalu tiap 60 dtk; retensi tiap jam.
	go traffic.NewSampler(database, engine).Run(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		slog.Info("menerima sinyal, mematikan server dengan rapi")
		if err := server.Shutdown(shutdownCtx); err != nil {
			// Melewati 10 detik: koneksi yang masih terbuka diputus paksa; dicatat agar terlihat di journal.
			slog.Warn("shutdown tidak selesai dengan rapi", "err", err)
		}
	}()
	slog.Info("wgpanel serve", "listen", cfg.Listen, "versi", buildVersion())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	slog.Info("server berhenti")
	return nil
}

// runApply membangun ulang semua interface dari DB lalu keluar (unit wgpanel-apply.service saat boot, N-4).
// Berbeda dengan serve, galat apa pun → exit 1, supaya systemd menandai unit boot gagal dan sysadmin melihatnya.
func runApply() error {
	ctx := context.Background()
	cfg, database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer closeDatabase(database)
	box, err := secret.NewBox(cfg.SecretKey, "wg-keys")
	if err != nil {
		return err
	}
	engine, err := wg.NewEngine()
	if err != nil {
		return err
	}
	defer func() { _ = engine.Close() }()
	applied, err := reconcile.New(database, engine, box, cfg.WGConfDir).ApplyAll(ctx)
	if err != nil {
		return err
	}
	slog.Info("apply selesai", "interface", applied)
	return nil
}

// runCreateAdmin membuat akun admin baru. Password dibaca dua kali dari terminal tanpa gema, tidak pernah
// dari argv (argv terlihat di `ps` dan riwayat shell). Username yang sudah ada → galat, exit 1; akun lama
// tidak pernah ditimpa. Galat juga bila stdin bukan terminal atau kedua password berbeda.
func runCreateAdmin(args []string) error {
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	username := flags.String("username", "", "username admin baru (wajib)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	name := strings.TrimSpace(*username)
	if name == "" {
		return errors.New("-username wajib diisi")
	}
	ctx := context.Background()
	_, database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer closeDatabase(database)
	queries := sqlc.New(database)
	// Dicek sebelum bertanya password supaya admin tidak mengetik dua kali untuk hasil yang pasti gagal.
	// UNIQUE di kolom username tetap menjadi penjaga terakhir bila ada dua proses bersamaan.
	if _, err := queries.GetUserByUsername(ctx, name); err == nil {
		return fmt.Errorf("username sudah dipakai: %s", name)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	stdin := int(os.Stdin.Fd())
	if !term.IsTerminal(stdin) {
		return errors.New("password harus diketik di terminal (stdin bukan terminal)")
	}
	password, err := readPassword(stdin, "Password: ")
	if err != nil {
		return err
	}
	confirmation, err := readPassword(stdin, "Ulangi password: ")
	if err != nil {
		return err
	}
	if password != confirmation {
		return errors.New("kedua password tidak sama")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	user, err := queries.CreateUser(ctx, sqlc.CreateUserParams{
		Username: name, PasswordHash: hash, Role: "admin", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return fmt.Errorf("username sudah dipakai: %s", name)
		}
		return err
	}
	slog.Info("admin dibuat", "user_id", user.ID, "username", user.Username)
	return nil
}

// readPassword menampilkan prompt ke stderr lalu membaca satu baris tanpa gema.
func readPassword(fd int, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	raw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("membaca password: %w", err)
	}
	return string(raw), nil
}

// closeDatabase menutup DB saat proses berakhir. Galatnya dicatat, bukan diabaikan: Close pada SQLite WAL
// menjalankan checkpoint terakhir, dan kegagalannya (mis. disk penuh) perlu terlihat di journal.
func closeDatabase(database *sql.DB) {
	if err := database.Close(); err != nil {
		slog.Error("gagal menutup basis data", "err", err)
	}
}

// buildVersion membaca revisi VCS yang ditanam go build, untuk membuktikan biner = commit.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	revision, modified := "dev", ""
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			if setting.Value == "true" {
				modified = "-dirty"
			}
		}
	}
	return revision + modified
}
