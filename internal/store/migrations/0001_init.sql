CREATE TABLE torrents (
  id       TEXT PRIMARY KEY,
  hash     TEXT NOT NULL UNIQUE,
  name     TEXT NOT NULL DEFAULT '',
  phase    TEXT NOT NULL,
  added_at INTEGER NOT NULL,
  magnet   TEXT NOT NULL DEFAULT '',
  error    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE links (
  token      TEXT PRIMARY KEY,
  torrent_id TEXT NOT NULL REFERENCES torrents(id) ON DELETE CASCADE,
  file_index INTEGER NOT NULL,
  path       TEXT NOT NULL,
  bytes      INTEGER NOT NULL,
  UNIQUE (torrent_id, file_index)
);

CREATE INDEX idx_torrents_added ON torrents(added_at DESC);
