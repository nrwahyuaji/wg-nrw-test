-- Query tag (PLAN 12.1). Relasi peer_tags ditulis clear+reinsert di handler.

-- name: ListTags :many
SELECT * FROM tags ORDER BY name;

-- name: GetTagByName :one
SELECT * FROM tags WHERE name = ? COLLATE NOCASE;

-- name: CreateTag :one
INSERT INTO tags (name) VALUES (?) RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = ?;

-- name: ListPeerTags :many
SELECT t.name FROM tags t JOIN peer_tags pt ON pt.tag_id = t.id WHERE pt.peer_id = ? ORDER BY t.name;

-- name: ListAllPeerTags :many
SELECT pt.peer_id, t.name FROM peer_tags pt JOIN tags t ON t.id = pt.tag_id ORDER BY pt.peer_id, t.name;

-- name: DeletePeerTags :exec
DELETE FROM peer_tags WHERE peer_id = ?;

-- name: AddPeerTag :exec
INSERT OR IGNORE INTO peer_tags (peer_id, tag_id) VALUES (?, ?);
