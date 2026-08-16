package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestTorrentIndexerRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tor := Torrent{ID: "AAAAAAAAAAAAA", Hash: "aabbcc", Name: "one", Phase: PhaseAdded, AddedAt: 1, Indexer: "TorrentLeech"}
	if err := s.InsertTorrent(ctx, tor); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Every read path must carry the indexer, since cleanup reads through
	// AllTorrents and the admin listing through ListTorrents.
	byID, err := s.TorrentByID(ctx, tor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byID.Indexer != "TorrentLeech" {
		t.Errorf("TorrentByID indexer = %q, want %q", byID.Indexer, "TorrentLeech")
	}
	byHash, err := s.TorrentByHash(ctx, tor.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if byHash.Indexer != "TorrentLeech" {
		t.Errorf("TorrentByHash indexer = %q, want %q", byHash.Indexer, "TorrentLeech")
	}
	all, err := s.AllTorrents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Indexer != "TorrentLeech" {
		t.Errorf("AllTorrents = %+v, want the indexer preserved", all)
	}
	listed, _, err := s.ListTorrents(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Indexer != "TorrentLeech" {
		t.Errorf("ListTorrents = %+v, want the indexer preserved", listed)
	}
}

// A row inserted without an indexer reads back as empty (the migration's
// default), which is what pre-migration rows look like.
func TestTorrentIndexerDefaultsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.InsertTorrent(ctx, Torrent{ID: "AAAAAAAAAAAAA", Hash: "aabbcc", Phase: PhaseAdded}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.TorrentByID(ctx, "AAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	if got.Indexer != "" {
		t.Errorf("indexer = %q, want empty", got.Indexer)
	}
}

// The risky half of the migration is the upgrade path: an existing
// database with rows in it must gain the column rather than only fresh
// ones getting it. The pre-0003 state is reconstructed by dropping the
// column and its migration marker, then reopening.
func TestMigrationAddsIndexerColumnToExistingDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	ctx := context.Background()

	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.InsertTorrent(ctx, Torrent{ID: "AAAAAAAAAAAAA", Hash: "aabbcc", Phase: PhaseAdded}); err != nil {
		t.Fatal(err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.db.Exec(`ALTER TABLE torrents DROP COLUMN indexer`); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if _, err := s2.db.Exec(`DELETE FROM schema_migrations WHERE version LIKE '%0003%'`); err != nil {
		t.Fatalf("clear migration marker: %v", err)
	}
	s2.Close()

	s3, err := Open(path)
	if err != nil {
		t.Fatalf("reopen (re-run migration): %v", err)
	}
	defer s3.Close()

	got, err := s3.TorrentByID(ctx, "AAAAAAAAAAAAA")
	if err != nil {
		t.Fatalf("read migrated row: %v", err)
	}
	if got.Indexer != "" {
		t.Errorf("indexer = %q, want empty for a pre-migration row", got.Indexer)
	}
	// And the column is writable afterwards.
	if err := s3.SetTorrentIndexer(ctx, "AAAAAAAAAAAAA", "Nyaa"); err != nil {
		t.Fatalf("set indexer after migration: %v", err)
	}
}

func TestSetTorrentIndexer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.InsertTorrent(ctx, Torrent{ID: "AAAAAAAAAAAAA", Hash: "aabbcc", Phase: PhaseAdded}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := s.SetTorrentIndexer(ctx, "AAAAAAAAAAAAA", "Nyaa"); err != nil {
		t.Fatalf("set indexer: %v", err)
	}
	got, err := s.TorrentByID(ctx, "AAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	if got.Indexer != "Nyaa" {
		t.Errorf("indexer = %q, want %q", got.Indexer, "Nyaa")
	}

	if err := s.SetTorrentIndexer(ctx, "missing", "Nyaa"); !errors.Is(err, ErrNotFound) {
		t.Errorf("set indexer on missing row = %v, want ErrNotFound", err)
	}
}
