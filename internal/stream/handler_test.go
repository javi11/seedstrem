package stream

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/config"
	"github.com/javib/seedstrem/internal/deluge/fake"
	"github.com/javib/seedstrem/internal/playsession"
	"github.com/javib/seedstrem/internal/store"
	"github.com/javib/seedstrem/internal/torrents"
)

const (
	pieceSize = 1024
	fileSize  = 4 * pieceSize
	testToken = "streamtoken00000000000000000000a"
)

type streamEnv struct {
	handler http.Handler
	fake    *fake.Server
	avail   *Availability
	content []byte
	dir     string
}

// newStreamEnv creates a torrent with one 4-piece file on disk plus a
// link token for it. Piece states start as given.
func newStreamEnv(t *testing.T, pieceStates []int, fileProgress float64) *streamEnv {
	t.Helper()

	dir := t.TempDir()
	content := make([]byte, fileSize)
	if _, err := rand.Read(content); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "movie.mkv"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	f := fake.New()
	f.Put(&fake.Torrent{
		Hash: testHash, Name: "movie.mkv", State: "downloading",
		SavePath:  "/downloads",
		PieceSize: pieceSize, PieceStates: pieceStates,
		Files: []fake.File{{Name: "movie.mkv", Size: fileSize, Priority: 1, Progress: fileProgress}},
	})

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	if err := st.InsertTorrent(ctx, store.Torrent{ID: "STREAMTEST000", Hash: testHash, Phase: store.PhaseSelected, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertLinks(ctx, []store.Link{{Token: testToken, TorrentID: "STREAMTEST000", FileIndex: 0, Path: "movie.mkv", Bytes: fileSize}}); err != nil {
		t.Fatal(err)
	}

	avail := NewAvailability(f)
	resolver := NewResolver(f, func() []config.Mapping {
		return []config.Mapping{{Remote: "/downloads", Local: dir}}
	})
	svc := torrents.New(st, f, func() torrents.Settings { return torrents.Settings{} }, nil)
	settings := func() Settings { return Settings{WaitTimeout: 5 * time.Second, ReadChunk: pieceSize} }
	h := NewHandler(st, f, svc, resolver, avail, playsession.New(), settings, nil)

	return &streamEnv{handler: h.Router(), fake: f, avail: avail, content: content, dir: dir}
}

func (e *streamEnv) get(t *testing.T, rangeHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/"+testToken+"/movie.mkv", nil)
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	return w
}

func TestServeCompletedFile(t *testing.T) {
	e := newStreamEnv(t, []int{2, 2, 2, 2}, 1)

	w := e.get(t, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !bytes.Equal(w.Body.Bytes(), e.content) {
		t.Error("full body mismatch")
	}
	if got := w.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q", got)
	}
	if got := w.Header().Get("Content-Type"); got != "video/x-matroska" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestServeRangeOnCompletedFile(t *testing.T) {
	e := newStreamEnv(t, []int{2, 2, 2, 2}, 1)

	w := e.get(t, "bytes=100-299")
	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d", w.Code)
	}
	if !bytes.Equal(w.Body.Bytes(), e.content[100:300]) {
		t.Error("range body mismatch")
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 100-299/4096" {
		t.Errorf("Content-Range = %q", got)
	}
}

func TestServeRangeOnPartialFileWithAvailablePieces(t *testing.T) {
	// Pieces 0 and 1 downloaded; request stays inside them.
	e := newStreamEnv(t, []int{2, 2, 0, 0}, 0.5)

	w := e.get(t, "bytes=0-2047")
	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), e.content[:2048]) {
		t.Error("partial range body mismatch")
	}
}

func TestServeWaitsForPiecesArrivingMidRequest(t *testing.T) {
	// Only piece 0 available; request spans all 4 pieces. Pieces flip
	// to downloaded as the availability poller sleeps.
	e := newStreamEnv(t, []int{2, 0, 0, 0}, 0.25)

	now := time.Unix(1000, 0)
	e.avail.now = func() time.Time { return now }
	polls := 0
	e.avail.sleep = func(_ context.Context, d time.Duration) error {
		now = now.Add(d)
		polls++
		e.fake.Update(testHash, func(tor *fake.Torrent) {
			// One more piece per poll.
			for i, st := range tor.PieceStates {
				if st != 2 {
					tor.PieceStates[i] = 2
					break
				}
			}
		})
		return nil
	}

	w := e.get(t, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), e.content) {
		t.Error("body mismatch after in-flight piece arrival")
	}
	if polls == 0 {
		t.Error("expected the reader to poll for pieces")
	}
}

func TestServeTimesOutWith503(t *testing.T) {
	e := newStreamEnv(t, []int{0, 0, 0, 0}, 0)

	now := time.Unix(1000, 0)
	e.avail.now = func() time.Time { return now }
	e.avail.sleep = func(_ context.Context, d time.Duration) error {
		now = now.Add(d)
		return nil
	}

	w := e.get(t, "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "10" {
		t.Errorf("Retry-After = %q; want 10", got)
	}
}

func TestServeSeekAheadWaitsOnlyForRequestedPieces(t *testing.T) {
	// Last piece available, everything before missing: a request for the
	// file tail must succeed without waiting for earlier pieces.
	e := newStreamEnv(t, []int{0, 0, 0, 2}, 0.25)

	w := e.get(t, "bytes=3072-4095")
	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), e.content[3072:]) {
		t.Error("tail range mismatch")
	}
}

func TestServeSuffixRange(t *testing.T) {
	e := newStreamEnv(t, []int{0, 0, 0, 2}, 0.25)

	w := e.get(t, "bytes=-1024")
	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d", w.Code)
	}
	if !bytes.Equal(w.Body.Bytes(), e.content[3072:]) {
		t.Error("suffix range mismatch")
	}
}

func TestServeUnknownToken(t *testing.T) {
	e := newStreamEnv(t, []int{2, 2, 2, 2}, 1)
	req := httptest.NewRequest(http.MethodGet, "/nosuchtoken/movie.mkv", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// newAbandonEnv builds a minimal Handler (no on-disk files/HTTP needed)
// for exercising checkAbandoned directly.
func newAbandonEnv(t *testing.T, progress, minProgressForCancel float64) (*Handler, store.Torrent, *fake.Server) {
	t.Helper()

	f := fake.New()
	f.Put(&fake.Torrent{Hash: testHash, State: "downloading", Progress: progress})

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	tor := store.Torrent{ID: "ABANDONTEST0", Hash: testHash, AddedAt: 1}
	if err := st.InsertTorrent(ctx, tor); err != nil {
		t.Fatal(err)
	}

	svc := torrents.New(st, f, func() torrents.Settings { return torrents.Settings{DeleteFilesOnRemove: true} }, nil)
	sessions := playsession.New()
	settings := func() Settings { return Settings{MinProgressForCancel: minProgressForCancel} }
	h := NewHandler(st, f, svc, NewResolver(f, func() []config.Mapping { return nil }), NewAvailability(f), sessions, settings, nil)

	return h, tor, f
}

func TestCheckAbandonedRemovesLowProgressTorrent(t *testing.T) {
	h, tor, f := newAbandonEnv(t, 0.02, 0.05)

	h.checkAbandoned(tor)

	if f.Get(testHash) != nil {
		t.Error("expected torrent removed from deluge")
	}
	if _, err := h.store.TorrentByID(context.Background(), tor.ID); err == nil {
		t.Error("expected torrent removed from store")
	}
}

func TestCheckAbandonedKeepsTorrentAboveThreshold(t *testing.T) {
	h, tor, f := newAbandonEnv(t, 0.5, 0.05)

	h.checkAbandoned(tor)

	if f.Get(testHash) == nil {
		t.Error("expected torrent to remain: progress above threshold")
	}
}

func TestCheckAbandonedDisabledWhenThresholdZero(t *testing.T) {
	h, tor, f := newAbandonEnv(t, 0, 0)

	h.checkAbandoned(tor)

	if f.Get(testHash) == nil {
		t.Error("expected torrent to remain: cancel-cleanup disabled")
	}
}

func TestCheckAbandonedSkipsWhenSomeoneElseIsWatching(t *testing.T) {
	h, tor, f := newAbandonEnv(t, 0.02, 0.05)
	end := h.sessions.Begin(tor.Hash) // another viewer started watching
	defer end()

	h.checkAbandoned(tor)

	if f.Get(testHash) == nil {
		t.Error("expected torrent to remain: another session is active")
	}
}

func TestRequestedStart(t *testing.T) {
	tests := []struct {
		header string
		want   int64
	}{
		{"", 0},
		{"bytes=100-", 100},
		{"bytes=100-200", 100},
		{"bytes=-500", fileSize - 500},
		{"bytes=0-0", 0},
		{"bytes=100-200,300-400", 100},
		{"garbage", 0},
		{"bytes=99999999-", 0}, // out of range → let ServeContent 416
	}
	for _, tt := range tests {
		if got := requestedStart(tt.header, fileSize); got != tt.want {
			t.Errorf("requestedStart(%q) = %d; want %d", tt.header, got, tt.want)
		}
	}
}
