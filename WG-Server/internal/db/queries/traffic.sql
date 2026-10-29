-- Traffic sampler (DESIGN 6.1) and retention (DESIGN 6.2). ASCII comments only (sqlc offset bug with non-ASCII).

-- name: ListPeerCounters :many
SELECT peer_id, rx_bytes, tx_bytes, read_at FROM peer_counters;

-- name: UpsertPeerCounter :exec
INSERT INTO peer_counters (peer_id, rx_bytes, tx_bytes, read_at) VALUES (?, ?, ?, ?)
ON CONFLICT(peer_id) DO UPDATE SET rx_bytes = excluded.rx_bytes, tx_bytes = excluded.tx_bytes, read_at = excluded.read_at;

-- name: UpsertTrafficHour :exec
INSERT INTO traffic_hourly (peer_id, hour, rx_bytes, tx_bytes) VALUES (?, ?, ?, ?)
ON CONFLICT(peer_id, hour) DO UPDATE SET rx_bytes = rx_bytes + excluded.rx_bytes, tx_bytes = tx_bytes + excluded.tx_bytes;

-- name: AddQuotaUsed :exec
UPDATE peers SET quota_used_bytes = quota_used_bytes + ? WHERE id = ?;

-- name: DeleteTrafficBefore :execrows
DELETE FROM traffic_hourly WHERE hour < ?;

-- name: ListPeerKeys :many
-- Map public key to peer for all peers of enabled interfaces (one query per sampler round).
SELECT p.id, p.public_key, p.interface_id FROM peers p
JOIN interfaces i ON i.id = p.interface_id WHERE i.enabled = 1;

-- Chart queries (PLAN 8.2). Scope = one peer (peer_id) or all peers of one interface (interface_id);
-- the unused id is passed as 0, which never matches because ids start at 1.

-- name: TrafficByHour :many
SELECT t.hour AS bucket, CAST(SUM(t.rx_bytes) AS INTEGER) AS rx_bytes, CAST(SUM(t.tx_bytes) AS INTEGER) AS tx_bytes
FROM traffic_hourly t JOIN peers p ON p.id = t.peer_id
WHERE (p.id = sqlc.arg(peer_id) OR p.interface_id = sqlc.arg(interface_id)) AND t.hour >= sqlc.arg(since)
GROUP BY t.hour ORDER BY t.hour;

-- name: TrafficByDay :many
-- Day bucket = WITA calendar day index: (hour + 8*3600) / 86400, so midnight WITA (16:00 UTC) starts a new day.
SELECT CAST((t.hour + 28800) / 86400 AS INTEGER) AS bucket,
  CAST(SUM(t.rx_bytes) AS INTEGER) AS rx_bytes, CAST(SUM(t.tx_bytes) AS INTEGER) AS tx_bytes
FROM traffic_hourly t JOIN peers p ON p.id = t.peer_id
WHERE (p.id = sqlc.arg(peer_id) OR p.interface_id = sqlc.arg(interface_id)) AND t.hour >= sqlc.arg(since)
GROUP BY bucket ORDER BY bucket;

-- name: TrafficByPeerSince :many
-- Dashboard: per-peer totals since a moment, largest first (today total + top 5 from one query).
SELECT p.id, p.name, p.interface_id,
  CAST(SUM(t.rx_bytes) AS INTEGER) AS rx_bytes, CAST(SUM(t.tx_bytes) AS INTEGER) AS tx_bytes
FROM traffic_hourly t JOIN peers p ON p.id = t.peer_id
WHERE t.hour >= ?
GROUP BY p.id ORDER BY SUM(t.rx_bytes + t.tx_bytes) DESC, p.id;
