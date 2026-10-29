-- +goose Up
CREATE TABLE users (
  id               INTEGER PRIMARY KEY,
  username         TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash    TEXT NOT NULL,                       -- bcrypt cost 12
  role             TEXT NOT NULL CHECK (role IN ('admin','operator','user')),
  disabled         INTEGER NOT NULL DEFAULT 0,
  max_peers        INTEGER NOT NULL DEFAULT 0,          -- portal: kuota peer buatan sendiri
  totp_secret      BLOB,                                -- terenkripsi; NULL = 2FA mati
  totp_last_counter INTEGER NOT NULL DEFAULT 0,         -- cegah kode dipakai ulang
  created_at       INTEGER NOT NULL,
  updated_at       INTEGER NOT NULL
);

CREATE TABLE sessions (
  token_hash   TEXT PRIMARY KEY,                        -- sha256 hex; token asli hanya di cookie
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  stage        TEXT NOT NULL DEFAULT 'full' CHECK (stage IN ('full','totp_pending','totp_enroll')),
  ip           TEXT NOT NULL,
  user_agent   TEXT NOT NULL,
  created_at   INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  expires_at   INTEGER NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE INDEX sessions_expires ON sessions(expires_at);

CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
-- kunci yang dipakai: key_check (blob uji kunci, base64), traffic_retention_days, require_admin_2fa,
-- telegram_chat_id, telegram_events (JSON array), share_default_hours, last_monthly_reset (YYYY-MM)

CREATE TABLE interfaces (
  id                    INTEGER PRIMARY KEY,
  name                  TEXT NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 15),
  address               TEXT NOT NULL,                  -- CIDR server, mis. 10.8.0.1/24
  listen_port           INTEGER NOT NULL UNIQUE CHECK (listen_port BETWEEN 1 AND 65535),
  mtu                   INTEGER NOT NULL DEFAULT 0,     -- 0 = 1420
  private_key           BLOB NOT NULL,                  -- terenkripsi
  public_key            TEXT NOT NULL,
  endpoint_host         TEXT NOT NULL,                  -- ditulis ke config client
  client_dns            TEXT NOT NULL DEFAULT '',       -- dipisah koma
  client_allowed_ips    TEXT NOT NULL DEFAULT '0.0.0.0/0',
  client_keepalive      INTEGER NOT NULL DEFAULT 25,
  enabled               INTEGER NOT NULL DEFAULT 1,
  allow_user_self_service INTEGER NOT NULL DEFAULT 0,   -- portal boleh membuat peer di sini
  created_at            INTEGER NOT NULL,
  updated_at            INTEGER NOT NULL
);

CREATE TABLE peers (
  id               INTEGER PRIMARY KEY,
  interface_id     INTEGER NOT NULL REFERENCES interfaces(id) ON DELETE RESTRICT,
  owner_user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  name             TEXT NOT NULL,
  note             TEXT NOT NULL DEFAULT '',
  address          TEXT NOT NULL,                       -- IP tunnel /32
  public_key       TEXT NOT NULL UNIQUE,
  private_key      BLOB,                                -- terenkripsi; NULL bila public key ditempel admin
  preshared_key    BLOB,                                -- terenkripsi; NULL = tanpa PSK
  client_allowed_ips TEXT NOT NULL DEFAULT '',          -- '' = ikut interface
  site_subnets     TEXT NOT NULL DEFAULT '',            -- LAN di belakang peer (site-to-site), dipisah koma
  enabled          INTEGER NOT NULL DEFAULT 1,
  disabled_reason  TEXT NOT NULL DEFAULT '' CHECK (disabled_reason IN ('','manual','expired','quota')),
  expires_at       INTEGER,                             -- NULL = tanpa batas
  quota_bytes      INTEGER NOT NULL DEFAULT 0,          -- 0 = tanpa kuota
  quota_period     TEXT NOT NULL DEFAULT 'none' CHECK (quota_period IN ('none','monthly')),
  quota_used_bytes INTEGER NOT NULL DEFAULT 0,          -- dihitung sejak reset terakhir
  created_at       INTEGER NOT NULL,
  updated_at       INTEGER NOT NULL,
  UNIQUE (interface_id, address),
  UNIQUE (interface_id, name)
);
CREATE INDEX peers_owner ON peers(owner_user_id);
CREATE INDEX peers_expiry ON peers(expires_at) WHERE expires_at IS NOT NULL AND enabled = 1;

CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE);
CREATE TABLE peer_tags (
  peer_id INTEGER NOT NULL REFERENCES peers(id) ON DELETE CASCADE,
  tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (peer_id, tag_id)
);
CREATE INDEX peer_tags_tag ON peer_tags(tag_id);

CREATE TABLE audit_log (
  id         INTEGER PRIMARY KEY,
  at         INTEGER NOT NULL,
  user_id    INTEGER,                                   -- NULL = sistem/anonim
  username   TEXT NOT NULL,                             -- salinan agar tetap terbaca setelah user dihapus
  ip         TEXT NOT NULL,
  action     TEXT NOT NULL,                             -- mis. peer.create, auth.login_failed
  target     TEXT NOT NULL DEFAULT '',                  -- mis. peer:12
  detail     TEXT NOT NULL DEFAULT '{}'                 -- JSON yang SUDAH diredaksi
);
CREATE INDEX audit_at ON audit_log(at);
CREATE INDEX audit_action ON audit_log(action, at);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE peer_tags;
DROP TABLE tags;
DROP TABLE peers;
DROP TABLE interfaces;
DROP TABLE settings;
DROP TABLE sessions;
DROP TABLE users;
