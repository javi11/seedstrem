-- origin records how a torrent entered seedstrem: 'native' when
-- seedstrem added it, 'adopted' when it was discovered in the download
-- client by label. Only adopted rows may be removed when their label
-- disappears, so existing rows default to native and stay untouchable.
ALTER TABLE torrents ADD COLUMN origin TEXT NOT NULL DEFAULT 'native';

CREATE INDEX idx_torrents_origin ON torrents(origin);
