-- +goose Up
-- Tautan berbagi sekali pakai (PLAN 10.2). Token asli tidak disimpan; hanya hash SHA-256.
CREATE TABLE share_links (
  id          INTEGER PRIMARY KEY,
  peer_id     INTEGER NOT NULL REFERENCES peers(id) ON DELETE CASCADE,
  token_hash  TEXT NOT NULL UNIQUE,
  created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at  INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL,
  used_at     INTEGER,                                  -- NULL = belum dibuka
  revoked_at  INTEGER                                   -- NULL = belum dicabut
);
CREATE INDEX share_peer ON share_links(peer_id);
CREATE INDEX share_token ON share_links(token_hash);

-- +goose Down
DROP TABLE share_links;
