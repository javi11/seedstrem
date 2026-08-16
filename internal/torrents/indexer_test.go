package torrents

import (
	"context"
	"testing"
)

func TestEnsureAddedPersistsIndexer(t *testing.T) {
	svc, _, db := newService(t)
	ctx := context.Background()

	tor, err := svc.EnsureAdded(ctx, testMagnet("Movie"), nil, Selector{Indexer: "TorrentLeech"})
	if err != nil {
		t.Fatalf("ensure added: %v", err)
	}
	got, err := db.TorrentByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Indexer != "TorrentLeech" {
		t.Errorf("indexer = %q, want %q", got.Indexer, "TorrentLeech")
	}
}

// A torrent added before the indexer was known (a pre-migration row, or an
// RSS grab that carried none) picks one up on the next add that has it.
func TestEnsureAddedBackfillsIndexer(t *testing.T) {
	svc, _, db := newService(t)
	ctx := context.Background()

	tor, err := svc.EnsureAdded(ctx, testMagnet("Movie"), nil, Selector{})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	got, err := svc.EnsureAdded(ctx, testMagnet("Movie"), nil, Selector{Indexer: "Nyaa"})
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if got.ID != tor.ID {
		t.Fatalf("re-add created a new row: %q vs %q", got.ID, tor.ID)
	}
	if got.Indexer != "Nyaa" {
		t.Errorf("returned indexer = %q, want %q", got.Indexer, "Nyaa")
	}
	stored, err := db.TorrentByID(ctx, tor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Indexer != "Nyaa" {
		t.Errorf("stored indexer = %q, want %q", stored.Indexer, "Nyaa")
	}
}

// The first attribution wins: re-playing the same release found through a
// different indexer must not change how long cleanup keeps it seeding.
func TestEnsureAddedKeepsFirstIndexer(t *testing.T) {
	svc, _, db := newService(t)
	ctx := context.Background()

	tor, err := svc.EnsureAdded(ctx, testMagnet("Movie"), nil, Selector{Indexer: "TorrentLeech"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	got, err := svc.EnsureAdded(ctx, testMagnet("Movie"), nil, Selector{Indexer: "Nyaa"})
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if got.Indexer != "TorrentLeech" {
		t.Errorf("returned indexer = %q, want the original %q", got.Indexer, "TorrentLeech")
	}
	stored, err := db.TorrentByID(ctx, tor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Indexer != "TorrentLeech" {
		t.Errorf("stored indexer = %q, want the original %q", stored.Indexer, "TorrentLeech")
	}
}

// An add with no indexer must not clear one that is already recorded.
func TestEnsureAddedEmptyIndexerDoesNotClear(t *testing.T) {
	svc, _, db := newService(t)
	ctx := context.Background()

	tor, err := svc.EnsureAdded(ctx, testMagnet("Movie"), nil, Selector{Indexer: "TorrentLeech"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	if _, err := svc.EnsureAdded(ctx, testMagnet("Movie"), nil, Selector{}); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	stored, err := db.TorrentByID(ctx, tor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Indexer != "TorrentLeech" {
		t.Errorf("stored indexer = %q, want it preserved", stored.Indexer)
	}
}
