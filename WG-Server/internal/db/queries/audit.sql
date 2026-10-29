-- Query audit log (DESIGN 4, PLAN 10.1). detail sudah diredaksi sebelum INSERT (internal/audit).

-- name: InsertAudit :exec
INSERT INTO audit_log (at, user_id, username, ip, action, target, detail)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- Filter: action persis, username berawalan, rentang waktu inklusif; kosong = tanpa filter. Urut terbaru dulu.
-- name: ListAudit :many
SELECT * FROM audit_log
WHERE (action = CAST(sqlc.arg(action) AS TEXT) OR CAST(sqlc.arg(action) AS TEXT) = '')
  AND (username LIKE CAST(sqlc.arg(username) AS TEXT) || '%' OR CAST(sqlc.arg(username) AS TEXT) = '')
  AND at >= sqlc.arg(from_at) AND at <= sqlc.arg(to_at)
ORDER BY id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAudit :one
SELECT COUNT(*) FROM audit_log
WHERE (action = CAST(sqlc.arg(action) AS TEXT) OR CAST(sqlc.arg(action) AS TEXT) = '')
  AND (username LIKE CAST(sqlc.arg(username) AS TEXT) || '%' OR CAST(sqlc.arg(username) AS TEXT) = '')
  AND at >= sqlc.arg(from_at) AND at <= sqlc.arg(to_at);
