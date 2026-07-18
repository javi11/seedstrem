package stremio

import (
	"testing"
	"time"
)

func TestTorrentFileCachePutGet(t *testing.T) {
	c := newTorrentFileCache()
	c.put("abc", []byte("data"))

	if got := c.get("abc"); string(got) != "data" {
		t.Errorf("get = %q, want data", got)
	}
	if got := c.get("missing"); got != nil {
		t.Errorf("get(missing) = %q, want nil", got)
	}
	// Empty inputs are ignored.
	c.put("", []byte("x"))
	c.put("y", nil)
	if c.get("y") != nil {
		t.Error("nil bytes should not be stored")
	}
}

func TestTorrentFileCacheTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newTorrentFileCache()
	c.now = func() time.Time { return now }

	c.put("abc", []byte("data"))
	now = now.Add(torrentFileCacheTTL + time.Second)
	if got := c.get("abc"); got != nil {
		t.Errorf("expired entry returned %q, want nil", got)
	}
}

func TestTorrentFileCacheEvictsOldestOverCap(t *testing.T) {
	now := time.Unix(0, 0)
	c := newTorrentFileCache()
	c.now = func() time.Time { return now }

	// First inserted is the oldest; fill to capacity, then one more.
	c.put("oldest", []byte("o"))
	for i := 0; i < torrentFileCacheMax; i++ {
		now = now.Add(time.Second)
		c.put(string(rune('A'+i%26))+time.Duration(i).String(), []byte("x"))
	}
	if c.get("oldest") != nil {
		t.Error("oldest entry should have been evicted once over capacity")
	}
}
