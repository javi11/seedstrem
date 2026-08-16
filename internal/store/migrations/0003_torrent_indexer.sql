-- Persist the Prowlarr indexer a torrent was grabbed from, so cleanup can
-- apply a per-indexer seed time (private trackers impose minimum seed
-- times that public ones do not). Empty for rows added before this
-- migration and for torrents added outside a Prowlarr search; those fall
-- back to the global cleanup.seed_time.
ALTER TABLE torrents ADD COLUMN indexer TEXT NOT NULL DEFAULT '';
