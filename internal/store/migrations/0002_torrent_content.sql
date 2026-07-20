-- Persist the Stremio content identity a torrent was added for, so the
-- stream handler can surface already-downloaded / in-progress torrents as
-- high-priority streams for the same content. Mirrors meta.Query
-- (Source, ID, Season, Episode). Empty/zero for rows added before this
-- migration; they simply won't be matched until re-played.
ALTER TABLE torrents ADD COLUMN content_source TEXT NOT NULL DEFAULT '';
ALTER TABLE torrents ADD COLUMN content_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE torrents ADD COLUMN season INTEGER NOT NULL DEFAULT 0;
ALTER TABLE torrents ADD COLUMN episode INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_torrents_content ON torrents(content_source, content_ref, season, episode);
