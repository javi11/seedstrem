package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()
	s2, err := Open(path) // migrations must not re-apply
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}

func TestTorrentCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tor := Torrent{ID: "ABC123DEF4567", Hash: "aabbcc", Name: "test", Phase: PhaseAdded, AddedAt: 1000, Magnet: "magnet:?xt=..."}
	if err := s.InsertTorrent(ctx, tor); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.TorrentByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got != tor {
		t.Errorf("got %+v; want %+v", got, tor)
	}

	got, err = s.TorrentByHash(ctx, "aabbcc")
	if err != nil {
		t.Fatalf("by hash: %v", err)
	}
	if got.ID != tor.ID {
		t.Errorf("by hash got id %q; want %q", got.ID, tor.ID)
	}

	if err := s.SetTorrentPhase(ctx, tor.ID, PhaseSelected); err != nil {
		t.Fatalf("set phase: %v", err)
	}
	if err := s.SetTorrentName(ctx, tor.ID, "renamed"); err != nil {
		t.Fatalf("set name: %v", err)
	}
	if err := s.SetTorrentError(ctx, tor.ID, "boom"); err != nil {
		t.Fatalf("set error: %v", err)
	}
	got, _ = s.TorrentByID(ctx, tor.ID)
	if got.Phase != PhaseSelected || got.Name != "renamed" || got.Error != "boom" {
		t.Errorf("updates lost: %+v", got)
	}

	if err := s.DeleteTorrent(ctx, tor.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.TorrentByID(ctx, tor.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestTorrentContentRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tor := Torrent{
		ID: "CONTENT0000001", Hash: "hc", Phase: PhaseAdded, AddedAt: 1,
		ContentSource: "tt", ContentRef: "tt0944947", Season: 1, Episode: 5,
	}
	if err := s.InsertTorrent(ctx, tor); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.TorrentByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got != tor {
		t.Errorf("content columns not round-tripped: got %+v; want %+v", got, tor)
	}
}

func TestTorrentsByContent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rows := []Torrent{
		{ID: "MOVIE00000001", Hash: "m1", Phase: PhaseAdded, AddedAt: 10, ContentSource: "tt", ContentRef: "tt1375666"},
		{ID: "SER1E5000001", Hash: "s15", Phase: PhaseAdded, AddedAt: 20, ContentSource: "tt", ContentRef: "tt0944947", Season: 1, Episode: 5},
		{ID: "SER1E6000001", Hash: "s16", Phase: PhaseAdded, AddedAt: 30, ContentSource: "tt", ContentRef: "tt0944947", Season: 1, Episode: 6},
		{ID: "LEGACY0000001", Hash: "leg", Phase: PhaseAdded, AddedAt: 40}, // no content identity
	}
	for _, r := range rows {
		if err := s.InsertTorrent(ctx, r); err != nil {
			t.Fatalf("insert %s: %v", r.ID, err)
		}
	}

	tests := []struct {
		name            string
		source, ref     string
		season, episode int
		wantIDs         []string
	}{
		{"movie match", "tt", "tt1375666", 0, 0, []string{"MOVIE00000001"}},
		{"series exact episode", "tt", "tt0944947", 1, 5, []string{"SER1E5000001"}},
		{"series other episode excluded", "tt", "tt0944947", 1, 6, []string{"SER1E6000001"}},
		{"series missing episode", "tt", "tt0944947", 1, 9, nil},
		{"unknown ref", "tt", "tt9999999", 0, 0, nil},
		{"blank ref never matches legacy rows", "", "", 0, 0, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.TorrentsByContent(ctx, tc.source, tc.ref, tc.season, tc.episode)
			if err != nil {
				t.Fatalf("by content: %v", err)
			}
			var gotIDs []string
			for _, g := range got {
				gotIDs = append(gotIDs, g.ID)
			}
			if len(gotIDs) != len(tc.wantIDs) {
				t.Fatalf("got ids %v; want %v", gotIDs, tc.wantIDs)
			}
			for i := range gotIDs {
				if gotIDs[i] != tc.wantIDs[i] {
					t.Errorf("got ids %v; want %v", gotIDs, tc.wantIDs)
				}
			}
		})
	}
}

func TestSetTorrentContent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.InsertTorrent(ctx, Torrent{ID: "BACKFILL00001", Hash: "bf", Phase: PhaseAdded, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTorrentContent(ctx, "BACKFILL00001", "tt", "tt0111161", 0, 0); err != nil {
		t.Fatalf("set content: %v", err)
	}
	got, _ := s.TorrentByID(ctx, "BACKFILL00001")
	if got.ContentSource != "tt" || got.ContentRef != "tt0111161" {
		t.Errorf("backfill lost: %+v", got)
	}
	// Now findable by content.
	found, err := s.TorrentsByContent(ctx, "tt", "tt0111161", 0, 0)
	if err != nil || len(found) != 1 || found[0].ID != "BACKFILL00001" {
		t.Errorf("expected backfilled row findable, got %+v err=%v", found, err)
	}

	if err := s.SetTorrentContent(ctx, "missing", "tt", "x", 0, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetTorrentContent missing: want ErrNotFound, got %v", err)
	}
}

func TestNotFoundPaths(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.TorrentByID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("TorrentByID: want ErrNotFound, got %v", err)
	}
	if err := s.SetTorrentPhase(ctx, "missing", PhaseSelected); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetTorrentPhase: want ErrNotFound, got %v", err)
	}
	if err := s.DeleteTorrent(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteTorrent: want ErrNotFound, got %v", err)
	}
	if _, err := s.LinkByToken(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("LinkByToken: want ErrNotFound, got %v", err)
	}
}

func TestDuplicateHashRejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.InsertTorrent(ctx, Torrent{ID: "A", Hash: "h1", Phase: PhaseAdded, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertTorrent(ctx, Torrent{ID: "B", Hash: "h1", Phase: PhaseAdded, AddedAt: 2}); err == nil {
		t.Error("expected unique constraint error on duplicate hash")
	}
}

func TestListTorrentsPagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := range 5 {
		err := s.InsertTorrent(ctx, Torrent{
			ID: string(rune('A'+i)) + "0000000000000", Hash: string(rune('a' + i)),
			Phase: PhaseAdded, AddedAt: int64(100 + i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	page, total, err := s.ListTorrents(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("total = %d; want 5", total)
	}
	if len(page) != 2 {
		t.Fatalf("page size = %d; want 2", len(page))
	}
	// Newest first
	if page[0].AddedAt != 104 || page[1].AddedAt != 103 {
		t.Errorf("wrong order: %+v", page)
	}

	page2, _, err := s.ListTorrents(ctx, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 || page2[0].AddedAt != 100 {
		t.Errorf("last page wrong: %+v", page2)
	}
}

func TestLinksLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tor := Torrent{ID: "T", Hash: "h", Phase: PhaseSelected, AddedAt: 1}
	if err := s.InsertTorrent(ctx, tor); err != nil {
		t.Fatal(err)
	}

	links := []Link{
		{Token: "tok2", TorrentID: "T", FileIndex: 2, Path: "b.mkv", Bytes: 20},
		{Token: "tok0", TorrentID: "T", FileIndex: 0, Path: "a.mkv", Bytes: 10},
	}
	if err := s.InsertLinks(ctx, links); err != nil {
		t.Fatalf("insert links: %v", err)
	}

	got, err := s.LinkByToken(ctx, "tok0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "a.mkv" || got.Bytes != 10 {
		t.Errorf("link mismatch: %+v", got)
	}

	byTorrent, err := s.LinksByTorrent(ctx, "T")
	if err != nil {
		t.Fatal(err)
	}
	if len(byTorrent) != 2 || byTorrent[0].FileIndex != 0 || byTorrent[1].FileIndex != 2 {
		t.Errorf("links not ordered by file_index: %+v", byTorrent)
	}

	// Cascade on torrent delete
	if err := s.DeleteTorrent(ctx, "T"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkByToken(ctx, "tok0"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected cascade delete of links, got %v", err)
	}
}

func TestInsertLinksRollbackOnConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.InsertTorrent(ctx, Torrent{ID: "T", Hash: "h", Phase: PhaseSelected, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	// Second link duplicates file_index → whole batch must roll back.
	err := s.InsertLinks(ctx, []Link{
		{Token: "x", TorrentID: "T", FileIndex: 0, Path: "a", Bytes: 1},
		{Token: "y", TorrentID: "T", FileIndex: 0, Path: "b", Bytes: 2},
	})
	if err == nil {
		t.Fatal("expected constraint error")
	}
	if _, err := s.LinkByToken(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected rollback of first link, got %v", err)
	}
}
