package cleanup

import (
	"context"
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/playsession"
	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/qbit/fake"
	"github.com/javib/seedstrem/internal/store"
	"github.com/javib/seedstrem/internal/torrents"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"

func newCleanup(t *testing.T, seedTime time.Duration) (*Cleanup, *fake.Server, *store.Store, *playsession.Sessions) {
	t.Helper()
	fakeDC := fake.New()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	svc := torrents.New(db, fakeDC, func() torrents.Settings {
		return torrents.Settings{DeleteFilesOnRemove: true}
	}, nil)
	sessions := playsession.New()

	c := New(db, fakeDC, svc, sessions, func() Settings {
		return Settings{SeedTime: seedTime}
	}, nil, time.Hour)

	return c, fakeDC, db, sessions
}

func TestSweepRemovesTorrentPastSeedTime(t *testing.T) {
	c, fakeDC, db, _ := newCleanup(t, 24*time.Hour)
	ctx := context.Background()

	if err := db.InsertTorrent(ctx, store.Torrent{ID: "T1", Hash: testHash, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	fakeDC.Put(&fake.Torrent{
		Hash: testHash, State: qbit.StateSeeding,
		Progress: 1, SeedingTime: 48 * time.Hour,
	})

	if err := c.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if fakeDC.Get(testHash) != nil {
		t.Error("expected torrent removed from qbittorrent")
	}
	if _, err := db.TorrentByID(ctx, "T1"); err == nil {
		t.Error("expected torrent removed from store")
	}
}

func TestSweepKeepsTorrentUnderSeedTime(t *testing.T) {
	c, fakeDC, db, _ := newCleanup(t, 24*time.Hour)
	ctx := context.Background()

	if err := db.InsertTorrent(ctx, store.Torrent{ID: "T1", Hash: testHash, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	fakeDC.Put(&fake.Torrent{
		Hash: testHash, State: qbit.StateSeeding,
		Progress: 1, SeedingTime: 1 * time.Hour,
	})

	if err := c.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if fakeDC.Get(testHash) == nil {
		t.Error("expected torrent to remain in qbittorrent")
	}
	if _, err := db.TorrentByID(ctx, "T1"); err != nil {
		t.Errorf("expected torrent to remain in store, got %v", err)
	}
}

func TestSweepKeepsIncompleteTorrent(t *testing.T) {
	c, fakeDC, db, _ := newCleanup(t, 24*time.Hour)
	ctx := context.Background()

	if err := db.InsertTorrent(ctx, store.Torrent{ID: "T1", Hash: testHash, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	// Still downloading, but somehow racked up a lot of "seeding" time —
	// should never happen in practice, but progress < 1 must still block
	// removal.
	fakeDC.Put(&fake.Torrent{
		Hash: testHash, State: qbit.StateDownloading,
		Progress: 0.5, SeedingTime: 48 * time.Hour,
	})

	if err := c.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if fakeDC.Get(testHash) == nil {
		t.Error("expected incomplete torrent to remain")
	}
}

func TestSweepSkipsTorrentBeingWatched(t *testing.T) {
	c, fakeDC, db, sessions := newCleanup(t, 24*time.Hour)
	ctx := context.Background()

	if err := db.InsertTorrent(ctx, store.Torrent{ID: "T1", Hash: testHash, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	fakeDC.Put(&fake.Torrent{
		Hash: testHash, State: qbit.StateSeeding,
		Progress: 1, SeedingTime: 48 * time.Hour,
	})

	end := sessions.Begin(testHash)
	defer end()

	if err := c.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if fakeDC.Get(testHash) == nil {
		t.Error("expected torrent past seed time to remain while actively being watched")
	}
	if _, err := db.TorrentByID(ctx, "T1"); err != nil {
		t.Errorf("expected torrent to remain in store, got %v", err)
	}
}

func TestSweepDisabledWhenSeedTimeZero(t *testing.T) {
	c, fakeDC, db, _ := newCleanup(t, 0)
	ctx := context.Background()

	if err := db.InsertTorrent(ctx, store.Torrent{ID: "T1", Hash: testHash, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	fakeDC.Put(&fake.Torrent{
		Hash: testHash, State: qbit.StateSeeding,
		Progress: 1, SeedingTime: 1000 * time.Hour,
	})

	if err := c.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if fakeDC.Get(testHash) == nil {
		t.Error("expected torrent to remain when seed time cleanup disabled")
	}
}
