-- Pengaturan kunci-nilai (tabel settings). Daftar kunci yang dipakai ada di komentar 0001_init.sql.

-- name: GetSetting :one
SELECT value FROM settings WHERE key = ?;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;
