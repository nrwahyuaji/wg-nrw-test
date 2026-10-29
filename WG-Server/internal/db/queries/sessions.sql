-- Sesi login server-side (DESIGN 7.1). Kolom token_hash = sha256 token; token asli hanya di cookie.

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, stage, ip, user_agent, created_at, last_seen_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSessionByHash :one
SELECT * FROM sessions WHERE token_hash = ?;

-- name: TouchSession :exec
-- Hanya menulis bila last_seen_at sudah lebih dari 60 detik lalu, supaya tiap request tidak menulis DB.
UPDATE sessions SET last_seen_at = sqlc.arg(now) WHERE token_hash = sqlc.arg(token_hash) AND last_seen_at < sqlc.arg(now) - 60;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: DeleteUserSessions :exec
-- Mencabut semua sesi user kecuali satu (sesi yang sedang dipakai saat ganti password).
DELETE FROM sessions WHERE user_id = sqlc.arg(user_id) AND token_hash <> sqlc.arg(except_hash);

-- name: DeleteExpiredSessions :execrows
-- Sesi lewat batas absolut atau idle; dipanggil sweeper setiap jam.
DELETE FROM sessions WHERE expires_at <= sqlc.arg(now) OR last_seen_at <= sqlc.arg(idle_before);
