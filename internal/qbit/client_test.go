package qbit_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/qbit/fake"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"

func newClient(t *testing.T) (qbit.Client, *fake.Server) {
	t.Helper()
	f := fake.New()
	t.Cleanup(f.Close)
	return qbit.New(f.URL(), "admin", "pass"), f
}

func TestAddMagnetCreatesStoppedTorrent(t *testing.T) {
	c, f := newClient(t)
	ctx := context.Background()

	magnet := "magnet:?xt=urn:btih:" + testHash + "&dn=test"
	err := c.AddMagnet(ctx, magnet, qbit.AddOptions{
		Category: "seedstrem", Stopped: true, SequentialDownload: true, FirstLastPiecePrio: true,
	})
	if err != nil {
		t.Fatalf("add magnet: %v", err)
	}

	tor := f.Get(testHash)
	if tor == nil {
		t.Fatal("torrent not created in fake")
	}
	if tor.State != "stoppedDL" {
		t.Errorf("state = %q; want stoppedDL", tor.State)
	}
	if tor.Category != "seedstrem" || !tor.SequentialDownload || !tor.FirstLastPiecePrio {
		t.Errorf("options not applied: %+v", tor)
	}
}

func TestTorrentLifecycle(t *testing.T) {
	c, f := newClient(t)
	ctx := context.Background()

	f.Put(&fake.Torrent{
		Hash: testHash, Name: "movie", State: "stoppedDL", Category: "seedstrem",
		SavePath: "/downloads", ContentPath: "/downloads/movie",
		PieceSize: 1 << 20, PieceStates: []int{2, 2, 1, 0},
		Files: []fake.File{
			{Name: "movie/movie.mkv", Size: 3 << 20, Priority: 1},
			{Name: "movie/sample.mkv", Size: 1 << 20, Priority: 1},
		},
	})

	tor, err := c.Torrent(ctx, testHash)
	if err != nil {
		t.Fatalf("get torrent: %v", err)
	}
	if tor.Name != "movie" || tor.State != "stoppedDL" {
		t.Errorf("torrent mismatch: %+v", tor)
	}
	if tor.TotalSize != 4<<20 {
		t.Errorf("total size = %d; want %d", tor.TotalSize, 4<<20)
	}

	files, err := c.Files(ctx, testHash)
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	if len(files) != 2 || files[0].Name != "movie/movie.mkv" || files[1].Index != 1 {
		t.Errorf("files mismatch: %+v", files)
	}
	if files[0].PieceRange != [2]int{0, 2} || files[1].PieceRange != [2]int{3, 3} {
		t.Errorf("piece ranges wrong: %+v %+v", files[0].PieceRange, files[1].PieceRange)
	}

	props, err := c.Properties(ctx, testHash)
	if err != nil {
		t.Fatalf("properties: %v", err)
	}
	if props.PieceSize != 1<<20 || props.PiecesNum != 4 {
		t.Errorf("properties mismatch: %+v", props)
	}

	states, err := c.PieceStates(ctx, testHash)
	if err != nil {
		t.Fatalf("piece states: %v", err)
	}
	want := []qbit.PieceState{qbit.PieceHave, qbit.PieceHave, qbit.PieceDownloading, qbit.PieceMissing}
	for i, st := range states {
		if st != want[i] {
			t.Errorf("piece %d = %v; want %v", i, st, want[i])
		}
	}

	if err := c.SetFilePriority(ctx, testHash, []int{1}, 0); err != nil {
		t.Fatalf("set file priority: %v", err)
	}
	if got := f.Get(testHash).Files[1].Priority; got != 0 {
		t.Errorf("priority not applied: %d", got)
	}

	if err := c.Start(ctx, testHash); err != nil {
		t.Fatalf("start: %v", err)
	}
	if got := f.Get(testHash).State; got != "downloading" {
		t.Errorf("state after start = %q; want downloading", got)
	}

	if err := c.Delete(ctx, testHash, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if f.Get(testHash) != nil {
		t.Error("torrent still present after delete")
	}

	calls := strings.Join(f.Calls(), "\n")
	if !strings.Contains(calls, "filePrio hash="+testHash+" ids=1 priority=0") {
		t.Errorf("filePrio call not recorded:\n%s", calls)
	}
	if !strings.Contains(calls, "deleteFiles=true") {
		t.Errorf("delete call not recorded:\n%s", calls)
	}
}

func TestTorrentNotFound(t *testing.T) {
	c, _ := newClient(t)
	_, err := c.Torrent(context.Background(), "ffffffffffffffffffffffffffffffffffffffff")
	if !errors.Is(err, qbit.ErrTorrentNotFound) {
		t.Errorf("want ErrTorrentNotFound, got %v", err)
	}
}

func TestTorrentsByCategory(t *testing.T) {
	c, f := newClient(t)
	ctx := context.Background()

	f.Put(&fake.Torrent{Hash: strings.Repeat("a", 40), Category: "seedstrem", State: "downloading"})
	f.Put(&fake.Torrent{Hash: strings.Repeat("b", 40), Category: "other", State: "downloading"})

	list, err := c.Torrents(ctx, "seedstrem")
	if err != nil {
		t.Fatalf("torrents: %v", err)
	}
	if len(list) != 1 || list[0].Hash != strings.Repeat("a", 40) {
		t.Errorf("category filter failed: %+v", list)
	}
}

func TestAppPreferencesAndVersion(t *testing.T) {
	c, f := newClient(t)
	ctx := context.Background()

	f.SetPrefs(fake.Prefs{TempPath: "/incomplete", TempPathEnabled: true, IncompleteFilesExt: true})

	prefs, err := c.AppPreferences(ctx)
	if err != nil {
		t.Fatalf("app preferences: %v", err)
	}
	if prefs.TempPath != "/incomplete" || !prefs.TempPathEnabled || !prefs.IncompleteFilesExt {
		t.Errorf("prefs mismatch: %+v", prefs)
	}

	v, err := c.Version(ctx)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != "v5.0.0" {
		t.Errorf("version = %q", v)
	}
}
