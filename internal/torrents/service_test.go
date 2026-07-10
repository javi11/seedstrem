package torrents

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/qbit/fake"
	"github.com/javib/seedstrem/internal/store"
)

// 40-char hex infohash → magnet. metainfo.FromMagnet and the fake both
// lowercase it, so the store mapping and qbit lookups align.
const testHash = "0123456789abcdef0123456789abcdef01234567"

func testMagnet(name string) string {
	return "magnet:?xt=urn:btih:" + testHash + "&dn=" + name
}

func newService(t *testing.T) (*Service, *fake.Server, *store.Store) {
	t.Helper()
	fakeQB := fake.New()
	t.Cleanup(fakeQB.Close)

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	qb := qbit.New(fakeQB.URL(), "admin", "adminpass")
	svc := New(db, qb, func() Settings {
		return Settings{Category: "seedstrem", MetadataTimeout: 2 * time.Second}
	}, nil)
	return svc, fakeQB, db
}

func TestResolveIdempotent(t *testing.T) {
	svc, fakeQB, db := newService(t)
	ctx := context.Background()

	// Pre-seed the fake so metadata is immediately available with a
	// season pack; add() won't overwrite an existing hash.
	fakeQB.Put(&fake.Torrent{
		Hash:  testHash,
		State: qbit.StateStoppedDL,
		Files: []fake.File{
			{Name: "Show.S01E04.1080p.mkv", Size: 500 << 20},
			{Name: "Show.S01E05.1080p.mkv", Size: 480 << 20},
		},
	})

	sel := Selector{IsSeries: true, Season: 1, Episode: 5}
	link1, err := svc.Resolve(ctx, testMagnet("Show.S01"), sel)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if link1.FileIndex != 1 {
		t.Errorf("picked file index = %d, want 1", link1.FileIndex)
	}

	link2, err := svc.Resolve(ctx, testMagnet("Show.S01"), sel)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if link2.Token != link1.Token {
		t.Errorf("resolve not idempotent: tokens %q vs %q", link1.Token, link2.Token)
	}

	// Torrent added exactly once, stopped, sequential + first/last prio.
	var addCalls int
	for _, c := range fakeQB.Calls() {
		if strings.HasPrefix(c, "add magnet=") {
			addCalls++
			for _, want := range []string{"stopped=true", "seq=true", "flp=true"} {
				if !strings.Contains(c, want) {
					t.Errorf("add call %q missing %q", c, want)
				}
			}
		}
	}
	if addCalls != 1 {
		t.Errorf("magnet added %d times, want 1", addCalls)
	}

	// Phase advanced to selected and the link persists.
	tor, err := db.TorrentByHash(ctx, testHash)
	if err != nil {
		t.Fatalf("torrent by hash: %v", err)
	}
	if tor.Phase != store.PhaseSelected {
		t.Errorf("phase = %q, want %q", tor.Phase, store.PhaseSelected)
	}
	if _, err := db.LinkByToken(ctx, link1.Token); err != nil {
		t.Errorf("link not found by token: %v", err)
	}
}

func TestWaitForMetadataTimeout(t *testing.T) {
	svc, fakeQB, _ := newService(t)
	// Torrent exists but never resolves files.
	fakeQB.Put(&fake.Torrent{Hash: testHash, State: qbit.StateMetaDL})

	// Speed up the poll loop.
	svc.sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }

	_, err := svc.WaitForMetadata(context.Background(), testHash, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
