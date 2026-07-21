package store

import (
	"context"
	"testing"
)

func TestTorrentsByHashes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seed := []Torrent{
		{ID: "AAAAAAAAAAAAA", Hash: "aabbcc", Name: "one", Phase: PhaseAdded, AddedAt: 1},
		{ID: "BBBBBBBBBBBBB", Hash: "ddeeff", Name: "two", Phase: PhaseAdded, AddedAt: 2},
	}
	for _, tor := range seed {
		if err := s.InsertTorrent(ctx, tor); err != nil {
			t.Fatalf("insert %s: %v", tor.ID, err)
		}
	}

	// Mixed hit/miss, and an uppercase query hash to confirm
	// case-insensitive matching keyed by lowercase.
	got, err := s.TorrentsByHashes(ctx, []string{"AABBCC", "nope", "ddeeff"})
	if err != nil {
		t.Fatalf("by hashes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d matches; want 2 (%+v)", len(got), got)
	}
	if got["aabbcc"].ID != "AAAAAAAAAAAAA" {
		t.Errorf("aabbcc mapped to %q; want AAAAAAAAAAAAA", got["aabbcc"].ID)
	}
	if got["ddeeff"].ID != "BBBBBBBBBBBBB" {
		t.Errorf("ddeeff mapped to %q; want BBBBBBBBBBBBB", got["ddeeff"].ID)
	}
	if _, ok := got["nope"]; ok {
		t.Error("unexpected match for missing hash")
	}
}

func TestTorrentsByHashesEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.TorrentsByHashes(context.Background(), nil)
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d; want 0", len(got))
	}
}
