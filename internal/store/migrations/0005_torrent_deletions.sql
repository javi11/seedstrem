-- torrent_deletions is the 48h audit log of removed torrents: which code
-- path removed one, and the evidence behind that decision. Deliberately
-- NOT a foreign key to torrents — the row it describes is being deleted,
-- so a cascade would destroy the evidence along with it.
CREATE TABLE torrent_deletions (
  id            TEXT PRIMARY KEY,
  torrent_id    TEXT NOT NULL,
  hash          TEXT NOT NULL,
  name          TEXT NOT NULL DEFAULT '',
  indexer       TEXT NOT NULL DEFAULT '',
  origin        TEXT NOT NULL DEFAULT '',
  deleted_at    INTEGER NOT NULL,
  reason        TEXT NOT NULL,
  seeding_time  INTEGER NOT NULL DEFAULT 0,
  seed_limit    INTEGER NOT NULL DEFAULT 0,
  ratio         REAL NOT NULL DEFAULT 0,
  ratio_limit   REAL NOT NULL DEFAULT 0,
  progress      REAL NOT NULL DEFAULT 0,
  progress_limit REAL NOT NULL DEFAULT 0,
  files_deleted INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_torrent_deletions_at ON torrent_deletions(deleted_at DESC);
