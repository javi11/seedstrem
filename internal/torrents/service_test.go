package torrents

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/qbit/fake"
	"github.com/javib/seedstrem/internal/store"
)

// 40-char hex infohash → magnet. metainfo.FromMagnet and the fake both
// lowercase it, so the store mapping and qbittorrent lookups align.
const testHash = "0123456789abcdef0123456789abcdef01234567"

func testMagnet(name string) string {
	return "magnet:?xt=urn:btih:" + testHash + "&dn=" + name
}

func newService(t *testing.T) (*Service, *fake.Server, *store.Store) {
	t.Helper()
	fakeDC := fake.New()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	svc := New(db, fakeDC, func() Settings {
		return Settings{MetadataTimeout: 2 * time.Second}
	}, nil)
	return svc, fakeDC, db
}

func TestResolveIdempotent(t *testing.T) {
	svc, fakeDC, db := newService(t)
	ctx := context.Background()

	// Pre-seed the fake so metadata is immediately available with a
	// season pack; add() won't overwrite an existing hash.
	fakeDC.Put(&fake.Torrent{
		Hash:  testHash,
		State: qbit.StatePaused,
		Files: []fake.File{
			{Name: "Show.S01E04.1080p.mkv", Size: 500 << 20},
			{Name: "Show.S01E05.1080p.mkv", Size: 480 << 20},
		},
	})

	sel := Selector{IsSeries: true, Season: 1, Episode: 5}
	link1, err := svc.Resolve(ctx, testMagnet("Show.S01"), nil, sel)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if link1.FileIndex != 1 {
		t.Errorf("picked file index = %d, want 1", link1.FileIndex)
	}

	link2, err := svc.Resolve(ctx, testMagnet("Show.S01"), nil, sel)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if link2.Token != link1.Token {
		t.Errorf("resolve not idempotent: tokens %q vs %q", link1.Token, link2.Token)
	}

	// Torrent added exactly once, running (so qBittorrent fetches
	// metadata), sequential + first/last prio.
	var addCalls int
	for _, c := range fakeDC.Calls() {
		if strings.HasPrefix(c, "add magnet=") {
			addCalls++
			for _, want := range []string{"stopped=false", "seq=true", "flp=true"} {
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

// Changing file priorities makes qBittorrent recompute piece priorities,
// which on several versions silently drops the first/last-piece boost even
// though the torrent's flag stays on. SelectAndLink must re-assert it
// (toggle off+on) after the SetFilePriority calls so the file's tail piece
// (MKV index) is fetched up front instead of in sequential order.
func TestSelectAndLinkReassertsFirstLastPiecePrio(t *testing.T) {
	svc, fakeDC, _ := newService(t)
	ctx := context.Background()

	fakeDC.Put(&fake.Torrent{
		Hash:               testHash,
		State:              qbit.StatePaused,
		FirstLastPiecePrio: true,
		Files: []fake.File{
			{Name: "Movie.2026.1080p.mkv", Size: 8 << 30},
		},
	})

	if _, err := svc.Resolve(ctx, testMagnet("Movie"), nil, Selector{}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var lastFilePrio, toggles []int
	for i, c := range fakeDC.Calls() {
		if strings.HasPrefix(c, "filePrio ") {
			lastFilePrio = append(lastFilePrio, i)
		}
		if strings.HasPrefix(c, "toggleFirstLastPiecePrio ") {
			toggles = append(toggles, i)
		}
	}
	if len(toggles) != 2 {
		t.Fatalf("toggleFirstLastPiecePrio called %d times, want 2 (off+on): calls=%v", len(toggles), fakeDC.Calls())
	}
	if len(lastFilePrio) == 0 || toggles[0] < lastFilePrio[len(lastFilePrio)-1] {
		t.Errorf("toggle must happen after the file-priority rewrite: calls=%v", fakeDC.Calls())
	}
	if tor := fakeDC.Get(testHash); !tor.FirstLastPiecePrio {
		t.Error("first/last piece priority must end enabled")
	}
}

func TestRemove(t *testing.T) {
	svc, fakeDC, db := newService(t)
	ctx := context.Background()

	fakeDC.Put(&fake.Torrent{Hash: testHash, State: qbit.StateSeeding})
	tor, err := svc.EnsureAdded(ctx, testMagnet("Show"), nil)
	if err != nil {
		t.Fatalf("ensure added: %v", err)
	}

	if err := svc.Remove(ctx, tor); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if fakeDC.Get(testHash) != nil {
		t.Error("torrent still present in qbittorrent after remove")
	}
	if _, err := db.TorrentByID(ctx, tor.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("torrent by id error = %v, want ErrNotFound", err)
	}
}

func TestRemoveMissingFromqBittorrentIsNotAnError(t *testing.T) {
	svc, fakeDC, db := newService(t)
	ctx := context.Background()

	fakeDC.Put(&fake.Torrent{Hash: testHash, State: qbit.StateSeeding})
	tor, err := svc.EnsureAdded(ctx, testMagnet("Show"), nil)
	if err != nil {
		t.Fatalf("ensure added: %v", err)
	}
	fakeDC.Remove(testHash) // simulate it having vanished from qBittorrent already

	if err := svc.Remove(ctx, tor); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := db.TorrentByID(ctx, tor.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("torrent by id error = %v, want ErrNotFound", err)
	}
}

func TestEnsureAddedUsesTorrentFileWhenPresent(t *testing.T) {
	svc, fakeDC, _ := newService(t)
	ctx := context.Background()

	// A minimal valid .torrent whose infohash matches testMagnet's hash
	// is not required here: EnsureAdded keys the store record off the
	// magnet's hash, while the fake keys off the .torrent's own hash. We
	// only assert which client method was used, so any valid .torrent
	// works.
	torrentFile := []byte("d4:infod6:lengthi1e4:name4:test12:piece lengthi1e6:pieces20:aaaaaaaaaaaaaaaaaaaaee")

	if _, err := svc.EnsureAdded(ctx, testMagnet("Show"), torrentFile); err != nil {
		t.Fatalf("ensure added: %v", err)
	}

	var fileAdds, magnetAdds int
	for _, c := range fakeDC.Calls() {
		if strings.HasPrefix(c, "add torrentfile=") {
			fileAdds++
			if !strings.Contains(c, "stopped=false") {
				t.Errorf("torrent-file add should be running: %q", c)
			}
		}
		if strings.HasPrefix(c, "add magnet=") {
			magnetAdds++
		}
	}
	if fileAdds != 1 || magnetAdds != 0 {
		t.Errorf("want 1 torrent-file add and 0 magnet adds, got file=%d magnet=%d", fileAdds, magnetAdds)
	}
}

func TestLiveProgress(t *testing.T) {
	svc, fakeDC, _ := newService(t)
	ctx := context.Background()
	fakeDC.Put(&fake.Torrent{Hash: testHash, State: qbit.StateDownloading, Progress: 0.5})

	got := svc.LiveProgress(ctx, []string{testHash, "ffffffffffffffffffffffffffffffffffffffff"})
	if got[testHash] != 0.5 {
		t.Errorf("progress[%s] = %v, want 0.5", testHash, got[testHash])
	}
	if _, ok := got["ffffffffffffffffffffffffffffffffffffffff"]; ok {
		t.Error("unknown hash should be absent from progress map")
	}

	// Empty input and nil receiver are safe no-ops.
	if len(svc.LiveProgress(ctx, nil)) != 0 {
		t.Error("empty hashes should yield empty map")
	}
	var nilSvc *Service
	if len(nilSvc.LiveProgress(ctx, []string{testHash})) != 0 {
		t.Error("nil service should yield empty map, not panic")
	}
}

func TestWaitForMetadataTimeout(t *testing.T) {
	svc, fakeDC, _ := newService(t)
	// Torrent exists but never resolves files.
	fakeDC.Put(&fake.Torrent{Hash: testHash, State: qbit.StateDownloading})

	// Speed up the poll loop.
	svc.sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }

	_, err := svc.WaitForMetadata(context.Background(), testHash, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
