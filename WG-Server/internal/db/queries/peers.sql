-- Query peer. Reconciler (Fase 4) hanya memasukkan peer aktif ke kernel.

-- name: ListActivePeersByInterface :many
SELECT * FROM peers WHERE interface_id = ? AND enabled = 1 ORDER BY id;

-- name: CountPeersByInterface :one
-- Semua peer (aktif maupun tidak): subnet terkunci selama ada peer apa pun (F-2.3).
SELECT COUNT(*) FROM peers WHERE interface_id = ?;

-- name: ListPeers :many
SELECT * FROM peers ORDER BY interface_id, id;

-- name: GetPeer :one
SELECT * FROM peers WHERE id = ?;

-- name: ListPeerAddresses :many
-- All peers (enabled or not) hold their IP: a disabled peer must not lose its address.
SELECT id, address FROM peers WHERE interface_id = ?;

-- name: ListSiteSubnets :many
-- Site-to-site subnets of every peer, checked for overlap across all interfaces (F-3.8).
SELECT id, site_subnets FROM peers WHERE site_subnets != '';

-- name: CreatePeer :one
INSERT INTO peers (interface_id, owner_user_id, name, note, address, public_key, private_key, preshared_key,
  client_allowed_ips, site_subnets, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdatePeer :one
UPDATE peers SET owner_user_id = ?, name = ?, note = ?, address = ?, client_allowed_ips = ?, site_subnets = ?,
  updated_at = ?
WHERE id = ?
RETURNING *;

-- name: SetPeerEnabled :one
UPDATE peers SET enabled = ?, disabled_reason = ?, updated_at = ? WHERE id = ?
RETURNING *;

-- name: RotatePeerKey :one
UPDATE peers SET public_key = ?, private_key = ?, preshared_key = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: RotateInterfaceKey :one
UPDATE interfaces SET private_key = ?, public_key = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeletePeer :exec
DELETE FROM peers WHERE id = ?;

-- name: ResetMonthlyQuota :exec
UPDATE peers SET quota_used_bytes = 0, updated_at = ? WHERE quota_period = 'monthly';

-- name: ReactivateQuotaPeers :exec
UPDATE peers SET enabled = 1, disabled_reason = '', updated_at = ? WHERE quota_period = 'monthly' AND disabled_reason = 'quota';

-- name: ListInterfacesByExpiredPeers :many
SELECT DISTINCT interface_id FROM peers WHERE enabled = 1 AND expires_at IS NOT NULL AND expires_at <= ?;

-- name: ListInterfacesByOverQuotaPeers :many
SELECT DISTINCT interface_id FROM peers WHERE enabled = 1 AND quota_bytes > 0 AND quota_used_bytes >= quota_bytes;

-- name: DisableExpiredPeers :exec
UPDATE peers SET enabled = 0, disabled_reason = 'expired', updated_at = ? WHERE enabled = 1 AND expires_at IS NOT NULL AND expires_at <= ?;

-- name: DisableOverQuotaPeers :exec
UPDATE peers SET enabled = 0, disabled_reason = 'quota', updated_at = ? WHERE enabled = 1 AND quota_bytes > 0 AND quota_used_bytes >= quota_bytes;

-- name: CountPeersByInterfaceGrouped :many
-- Dashboard: total and enabled peers per interface in one query (no N+1).
SELECT interface_id, COUNT(*) AS total, CAST(COALESCE(SUM(enabled), 0) AS INTEGER) AS enabled
FROM peers GROUP BY interface_id;

-- name: ListExpiringPeers :many
-- Dashboard: enabled peers expiring before the given unix time, soonest first.
SELECT id, name, interface_id, expires_at FROM peers
WHERE enabled = 1 AND expires_at IS NOT NULL AND expires_at <= ?
ORDER BY expires_at LIMIT 20;
