-- Query share links (PLAN 10.2). Token asli tidak pernah disimpan (hanya hash).

-- name: CreateShareLink :one
INSERT INTO share_links (peer_id, token_hash, created_by, created_at, expires_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetShareLinkByHash :one
SELECT * FROM share_links WHERE token_hash = ?;

-- name: MarkShareLinkUsed :execrows
-- Atomik: 0 baris = sudah dipakai/dicabut/kedaluwarsa.
UPDATE share_links SET used_at = ? WHERE id = ? AND used_at IS NULL AND revoked_at IS NULL;

-- name: RevokeShareLink :exec
UPDATE share_links SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL;

-- name: RevokeShareLinksByPeer :exec
-- Dipanggil saat rotasi key peer (Fase 12); semua tautan peer ini dicabut.
UPDATE share_links SET revoked_at = ? WHERE peer_id = ? AND revoked_at IS NULL;

-- name: GetShareLinkByID :one
SELECT * FROM share_links WHERE id = ?;

-- name: ListShareLinksByPeer :many
SELECT * FROM share_links WHERE peer_id = ? ORDER BY created_at DESC;

-- name: DeleteExpiredShareLinks :exec
-- Pembersih opsional; gerbang tidak mewajibkan.
DELETE FROM share_links WHERE expires_at < ? AND used_at IS NULL;
