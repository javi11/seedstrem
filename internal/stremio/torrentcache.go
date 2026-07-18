package stremio

import (
	"sync"
	"time"
)

// torrentFileCacheTTL bounds how long a fetched .torrent stays cached
// between a stream (browse) request and the play that follows it.
const torrentFileCacheTTL = 30 * time.Minute

// torrentFileCacheMax caps the number of cached .torrent files so a burst
// of searches cannot grow memory without bound. Oldest entries are
// evicted first.
const torrentFileCacheMax = 512

type torrentFileEntry struct {
	raw []byte
	at  time.Time
}

// torrentFileCache is a small, bounded, TTL cache mapping a torrent
// infohash to its raw .torrent bytes. It bridges the stateless
// stream→play flow: the .torrent fetched while building the stream list
// (see prowlarr magnet-less resolution) is stashed here so the play
// handler can add it directly to qBittorrent instead of a metadata-less
// magnet. Misses are harmless — the play flow falls back to the magnet.
type torrentFileCache struct {
	mu      sync.Mutex
	entries map[string]torrentFileEntry
	now     func() time.Time // injectable for tests
}

func newTorrentFileCache() *torrentFileCache {
	return &torrentFileCache{
		entries: map[string]torrentFileEntry{},
		now:     time.Now,
	}
}

// put stores raw bytes for hash. No-op for an empty hash or nil bytes.
func (c *torrentFileCache) put(hash string, raw []byte) {
	if hash == "" || len(raw) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked()
	if len(c.entries) >= torrentFileCacheMax {
		c.evictOldestLocked()
	}
	c.entries[hash] = torrentFileEntry{raw: raw, at: c.now()}
}

// get returns the cached bytes for hash, or nil if absent/expired. The
// entry is left in place so a retried resolve still finds it.
func (c *torrentFileCache) get(hash string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[hash]
	if !ok {
		return nil
	}
	if c.now().Sub(e.at) > torrentFileCacheTTL {
		delete(c.entries, hash)
		return nil
	}
	return e.raw
}

func (c *torrentFileCache) evictExpiredLocked() {
	for h, e := range c.entries {
		if c.now().Sub(e.at) > torrentFileCacheTTL {
			delete(c.entries, h)
		}
	}
}

func (c *torrentFileCache) evictOldestLocked() {
	var oldestHash string
	var oldestAt time.Time
	for h, e := range c.entries {
		if oldestHash == "" || e.at.Before(oldestAt) {
			oldestHash, oldestAt = h, e.at
		}
	}
	if oldestHash != "" {
		delete(c.entries, oldestHash)
	}
}
