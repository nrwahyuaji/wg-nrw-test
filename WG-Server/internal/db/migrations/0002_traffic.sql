-- +goose Up
-- Riwayat trafik per peer per jam (DESIGN 4, 6.1). hour = awal jam UTC (unix, kelipatan 3600).
CREATE TABLE traffic_hourly (
  peer_id    INTEGER NOT NULL REFERENCES peers(id) ON DELETE CASCADE,
  hour       INTEGER NOT NULL,
  rx_bytes   INTEGER NOT NULL DEFAULT 0,
  tx_bytes   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (peer_id, hour)
) WITHOUT ROWID;
CREATE INDEX traffic_hour ON traffic_hourly(hour);

-- Pembacaan kernel terakhir per peer, dasar perhitungan delta (traffic.Delta).
CREATE TABLE peer_counters (
  peer_id    INTEGER PRIMARY KEY REFERENCES peers(id) ON DELETE CASCADE,
  rx_bytes   INTEGER NOT NULL,
  tx_bytes   INTEGER NOT NULL,
  read_at    INTEGER NOT NULL
);

-- +goose Down
DROP TABLE peer_counters;
DROP TABLE traffic_hourly;
