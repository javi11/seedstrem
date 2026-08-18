package store

import (
	"context"
	"testing"
)

func TestOriginDefaultsToNative(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if err := st.InsertTorrent(ctx, Torrent{
		ID: "T1", Hash: "aaaa", Name: "native one", Phase: PhaseAdded, AddedAt: 100,
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := st.TorrentByHash(ctx, "aaaa")
	if err != nil {
		t.Fatalf("by hash: %v", err)
	}
	if got.Origin != OriginNative {
		t.Fatalf("origin = %q, want %q", got.Origin, OriginNative)
	}
}

func TestAdoptedTorrentsReturnsOnlyAdopted(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if err := st.InsertTorrent(ctx, Torrent{
		ID: "T1", Hash: "aaaa", Phase: PhaseAdded, AddedAt: 100, Origin: OriginNative,
	}); err != nil {
		t.Fatalf("insert native: %v", err)
	}
	if err := st.InsertTorrent(ctx, Torrent{
		ID: "T2", Hash: "bbbb", Phase: PhaseSelected, AddedAt: 200, Origin: OriginAdopted,
	}); err != nil {
		t.Fatalf("insert adopted: %v", err)
	}

	got, err := st.AdoptedTorrents(ctx)
	if err != nil {
		t.Fatalf("adopted: %v", err)
	}
	if len(got) != 1 || got[0].ID != "T2" {
		t.Fatalf("got %+v, want exactly the adopted row T2", got)
	}
}

func TestInsertTorrentEmptyOriginStoredAsNative(t *testing.T) {
	// Callers that predate this column leave Origin zero; the row must
	// still be a native row, never an empty-string origin that the
	// un-adopt path could not classify.
	ctx := context.Background()
	st := newTestStore(t)

	if err := st.InsertTorrent(ctx, Torrent{
		ID: "T1", Hash: "aaaa", Phase: PhaseAdded, AddedAt: 100, Origin: "",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	adopted, err := st.AdoptedTorrents(ctx)
	if err != nil {
		t.Fatalf("adopted: %v", err)
	}
	if len(adopted) != 0 {
		t.Fatalf("got %d adopted rows, want 0", len(adopted))
	}
}
