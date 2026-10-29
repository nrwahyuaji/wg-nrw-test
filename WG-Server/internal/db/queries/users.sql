-- Query akun panel. Dipakai login (Fase 3), create-admin, dan penjaga "admin terakhir" (Fase 13).

-- name: GetUserByUsername :one
-- Pencarian tidak peka huruf besar/kecil karena kolom username ber-COLLATE NOCASE.
SELECT * FROM users WHERE username = ?;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ?;

-- name: CreateUser :one
INSERT INTO users (username, password_hash, role, max_peers, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: CountActiveAdmins :one
-- Admin aktif = role admin dan tidak dinonaktifkan; dipakai menolak penghapusan/penurunan admin terakhir.
SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?;
