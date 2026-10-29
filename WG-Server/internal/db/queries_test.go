package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"wgpanel/internal/sqlc"
)

// TestQueries menjalankan setiap query sqlc dasar terhadap skema hasil migrasi sungguhan, supaya SQL
// yang hanya lolos kompilasi tetapi salah kolom/klausa ketahuan sekarang, bukan di Fase 3–5.
func TestQueries(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "uji.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	ctx := context.Background()
	if _, err := Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}
	queries := sqlc.New(database)

	created, err := queries.CreateUser(ctx, sqlc.CreateUserParams{
		Username: "Admin", PasswordHash: "h", Role: "admin", CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// COLLATE NOCASE: "admin" harus menemukan "Admin".
	byName, err := queries.GetUserByUsername(ctx, "admin")
	if err != nil || byName.ID != created.ID {
		t.Fatalf("GetUserByUsername: %+v %v", byName, err)
	}
	if byID, err := queries.GetUserByID(ctx, created.ID); err != nil || byID.Username != "Admin" {
		t.Fatalf("GetUserByID: %+v %v", byID, err)
	}
	if count, err := queries.CountActiveAdmins(ctx); err != nil || count != 1 {
		t.Fatalf("CountActiveAdmins: %d %v", count, err)
	}

	// Upsert dua kali pada kunci yang sama harus menimpa, bukan galat UNIQUE.
	for _, value := range []string{"30", "90"} {
		if err := queries.UpsertSetting(ctx, sqlc.UpsertSettingParams{Key: "traffic_retention_days", Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	if value, err := queries.GetSetting(ctx, "traffic_retention_days"); err != nil || value != "90" {
		t.Fatalf("GetSetting: %q %v", value, err)
	}
	if _, err := queries.GetSetting(ctx, "tidak-ada"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("kunci hilang harus sql.ErrNoRows, got %v", err)
	}

	result, err := database.Exec(`INSERT INTO interfaces (name, address, listen_port, private_key, public_key,
		endpoint_host, created_at, updated_at) VALUES ('wg0', '10.8.0.1/24', 51820, x'00', 'pub', 'vpn', 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	interfaceID, _ := result.LastInsertId()
	if list, err := queries.ListInterfaces(ctx); err != nil || len(list) != 1 || list[0].Name != "wg0" {
		t.Fatalf("ListInterfaces: %+v %v", list, err)
	}
	if one, err := queries.GetInterface(ctx, interfaceID); err != nil || one.ListenPort != 51820 {
		t.Fatalf("GetInterface: %+v %v", one, err)
	}

	// Hanya peer enabled = 1 yang boleh dikembalikan, berurutan id.
	for i, row := range []struct {
		name, address, key string
		enabled            int
	}{{"a", "10.8.0.2/32", "ka", 1}, {"b", "10.8.0.3/32", "kb", 0}, {"c", "10.8.0.4/32", "kc", 1}} {
		if _, err := database.Exec(`INSERT INTO peers (interface_id, name, address, public_key, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, interfaceID, row.name, row.address, row.key, row.enabled, i, i); err != nil {
			t.Fatal(err)
		}
	}
	active, err := queries.ListActivePeersByInterface(ctx, interfaceID)
	if err != nil || len(active) != 2 || active[0].Name != "a" || active[1].Name != "c" {
		t.Fatalf("ListActivePeersByInterface: %+v %v", active, err)
	}
}
