package stream

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/javib/seedstrem/internal/config"
	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/qbit/fake"
)

func TestRemap(t *testing.T) {
	mappings := []config.Mapping{
		{QBit: "/downloads", Local: "/data"},
		{QBit: "/downloads/movies", Local: "/mnt/movies"}, // longer prefix wins
	}

	tests := []struct {
		in   string
		want string
	}{
		{"/downloads/file.mkv", "/data/file.mkv"},
		{"/downloads/movies/file.mkv", "/mnt/movies/file.mkv"},
		{"/downloads", "/data"},
		{"/elsewhere/file.mkv", "/elsewhere/file.mkv"}, // passthrough
		{"/downloadsfoo/file.mkv", "/downloadsfoo/file.mkv"}, // not a path-segment match
	}
	for _, tt := range tests {
		if got := Remap(mappings, tt.in); got != tt.want {
			t.Errorf("Remap(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func newResolverEnv(t *testing.T, mappings []config.Mapping) (*Resolver, *fake.Server) {
	t.Helper()
	f := fake.New()
	t.Cleanup(f.Close)
	return NewResolver(qbit.New(f.URL(), "u", "p"), func() []config.Mapping { return mappings }), f
}

func TestFilePathMultiFileTorrent(t *testing.T) {
	local := t.TempDir()
	if err := os.MkdirAll(filepath.Join(local, "Movie"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(local, "Movie", "movie.mkv")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, _ := newResolverEnv(t, []config.Mapping{{QBit: "/downloads", Local: local}})

	info := qbit.TorrentInfo{Hash: testHash, SavePath: "/downloads", ContentPath: "/downloads/Movie"}
	file := qbit.FileInfo{Index: 0, Name: "Movie/movie.mkv", Size: 1}

	got, err := r.FilePath(context.Background(), info, file)
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	if got != target {
		t.Errorf("path = %q; want %q", got, target)
	}
}

func TestFilePathIncompleteExtension(t *testing.T) {
	local := t.TempDir()
	target := filepath.Join(local, "movie.mkv") + incompleteExt
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, _ := newResolverEnv(t, []config.Mapping{{QBit: "/downloads", Local: local}})
	info := qbit.TorrentInfo{Hash: testHash, SavePath: "/downloads", ContentPath: "/downloads/movie.mkv"}
	file := qbit.FileInfo{Index: 0, Name: "movie.mkv", Size: 1}

	got, err := r.FilePath(context.Background(), info, file)
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	if got != target {
		t.Errorf("path = %q; want %q", got, target)
	}
}

func TestFilePathContentPathRename(t *testing.T) {
	// Torrent renamed in qBittorrent: content_path differs from the
	// original folder in file.Name.
	local := t.TempDir()
	if err := os.MkdirAll(filepath.Join(local, "Renamed"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(local, "Renamed", "movie.mkv")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, _ := newResolverEnv(t, []config.Mapping{{QBit: "/downloads", Local: local}})
	info := qbit.TorrentInfo{Hash: testHash, SavePath: "/downloads", ContentPath: "/downloads/Renamed"}
	file := qbit.FileInfo{Index: 0, Name: "Original/movie.mkv", Size: 1}

	got, err := r.FilePath(context.Background(), info, file)
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	if got != target {
		t.Errorf("path = %q; want %q", got, target)
	}
}

func TestFilePathTempDir(t *testing.T) {
	localDone := t.TempDir()
	localTemp := t.TempDir()
	target := filepath.Join(localTemp, "movie.mkv")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, f := newResolverEnv(t, []config.Mapping{
		{QBit: "/downloads", Local: localDone},
		{QBit: "/incomplete", Local: localTemp},
	})
	f.SetPrefs(fake.Prefs{TempPath: "/incomplete", TempPathEnabled: true})

	info := qbit.TorrentInfo{Hash: testHash, SavePath: "/downloads", ContentPath: "/downloads/movie.mkv"}
	file := qbit.FileInfo{Index: 0, Name: "movie.mkv", Size: 1}

	got, err := r.FilePath(context.Background(), info, file)
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	if got != target {
		t.Errorf("path = %q; want %q", got, target)
	}
}

func TestFilePathRejectsTraversal(t *testing.T) {
	local := t.TempDir()
	// Plant a file outside the mapped root that a traversal would reach.
	outside := filepath.Join(filepath.Dir(local), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(outside) })

	r, _ := newResolverEnv(t, []config.Mapping{{QBit: "/downloads", Local: local}})
	info := qbit.TorrentInfo{Hash: testHash, SavePath: "/downloads", ContentPath: "/downloads"}
	file := qbit.FileInfo{Index: 0, Name: "../secret.txt", Size: 6}

	_, err := r.FilePath(context.Background(), info, file)
	if !errors.Is(err, ErrUnsafePath) {
		t.Errorf("want ErrUnsafePath for traversal name, got %v", err)
	}
}

func TestFilePathRejectsEscapeOutsideRoot(t *testing.T) {
	// Even without ".." in the file name, a candidate that resolves
	// outside every mapping root must not be served.
	local := t.TempDir()
	r, _ := newResolverEnv(t, []config.Mapping{{QBit: "/downloads", Local: local}})
	// content_path points somewhere unmapped; the remap leaves it as-is,
	// so it lands outside local and must be rejected.
	info := qbit.TorrentInfo{Hash: testHash, SavePath: "/elsewhere", ContentPath: "/elsewhere/movie.mkv"}
	file := qbit.FileInfo{Index: 0, Name: "movie.mkv", Size: 1}

	_, err := r.FilePath(context.Background(), info, file)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want not-found for unmapped path, got %v", err)
	}
}

func TestWithinAnyRoot(t *testing.T) {
	roots := []string{"/data", "/mnt/media"}
	cases := map[string]bool{
		"/data/movie.mkv":       true,
		"/data":                 true,
		"/mnt/media/a/b.mkv":    true,
		"/etc/passwd":           false,
		"/datax/movie.mkv":      false, // prefix but not a path segment
		"/mnt/mediafoo/x":       false,
	}
	for p, want := range cases {
		if got := withinAnyRoot(p, roots); got != want {
			t.Errorf("withinAnyRoot(%q) = %v; want %v", p, got, want)
		}
	}
	// No roots configured → permissive.
	if !withinAnyRoot("/anything", nil) {
		t.Error("empty roots should permit any path")
	}
}

func TestFilePathNotFound(t *testing.T) {
	r, _ := newResolverEnv(t, []config.Mapping{{QBit: "/downloads", Local: t.TempDir()}})
	info := qbit.TorrentInfo{Hash: testHash, SavePath: "/downloads"}
	file := qbit.FileInfo{Index: 0, Name: "missing.mkv", Size: 1}

	_, err := r.FilePath(context.Background(), info, file)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want ErrNotExist, got %v", err)
	}
}
