-- Query interface WireGuard. Urutan id supaya daftar dan rekonsiliasi deterministik.

-- name: ListInterfaces :many
SELECT * FROM interfaces ORDER BY id;

-- name: GetInterface :one
SELECT * FROM interfaces WHERE id = ?;

-- name: CreateInterface :one
INSERT INTO interfaces (name, address, listen_port, mtu, private_key, public_key, endpoint_host,
  client_dns, client_allowed_ips, client_keepalive, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateInterface :one
-- Kunci server tidak diubah di sini (rotasi kunci = Fase 12).
UPDATE interfaces SET name = ?, address = ?, listen_port = ?, mtu = ?, endpoint_host = ?, client_dns = ?,
  client_allowed_ips = ?, client_keepalive = ?, enabled = ?, updated_at = ?
WHERE id = ?
RETURNING *;
