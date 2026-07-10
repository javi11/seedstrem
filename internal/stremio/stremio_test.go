package stremio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/javib/seedstrem/internal/meta"
	"github.com/javib/seedstrem/internal/prowlarr"
	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/qbit/fake"
	"github.com/javib/seedstrem/internal/store"
	"github.com/javib/seedstrem/internal/torrents"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"

func testMagnet() string {
	return "magnet:?xt=urn:btih:" + testHash + "&dn=The.Matrix.1999.1080p"
}

// harness wires a Handler over fakes: cinemeta, prowlarr, and qBittorrent.
type harness struct {
	handler  *Handler
	server   *httptest.Server
	prowlarr *httptest.Server
	cinemeta *httptest.Server
	fakeQB   *fake.Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	cinemeta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/meta/movie/tt1375666") {
			w.Write([]byte(`{"meta":{"name":"The Matrix","releaseInfo":"1999"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(cinemeta.Close)

	prow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"title":"The Matrix 1999 1080p BluRay","magnetUrl":"` + testMagnet() + `","size":8589934592,"seeders":42,"protocol":"torrent","indexer":"idx1"},
			{"title":"The Matrix 1999 720p","infoHash":"ffffffffffffffffffffffffffffffffffffffff","size":2000000000,"seeders":10,"protocol":"torrent","indexer":"idx2"}
		]`))
	}))
	t.Cleanup(prow.Close)

	fakeQB := fake.New()
	t.Cleanup(fakeQB.Close)
	fakeQB.Put(&fake.Torrent{
		Hash:  testHash,
		State: qbit.StateStoppedDL,
		Files: []fake.File{{Name: "The.Matrix.1999.1080p.BluRay.mkv", Size: 8 << 30}},
	})

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	qb := qbit.New(fakeQB.URL(), "admin", "pass")
	svc := torrents.New(db, qb, func() torrents.Settings {
		return torrents.Settings{Category: "seedstrem", MetadataTimeout: 2 * time.Second}
	}, nil)

	metaClient := meta.New(cinemeta.URL)

	h := &harness{prowlarr: prow, cinemeta: cinemeta, fakeQB: fakeQB}
	h.handler = New(svc, metaClient, func() Settings {
		return Settings{
			ExternalURL: h.server.URL,
			Prowlarr:    ProwlarrSettings{URL: prow.URL, APIKey: "k", MovieCategories: []int{2000}},
			Addon:       AddonSettings{EnableMovies: true, EnableSeries: true},
			Filters:     prowlarr.Filters{MinSeeders: 1},
			MaxResults:  20,
		}
	}, "test", nil)

	// Mount under /stremio exactly as the production server does.
	root := chi.NewRouter()
	root.Mount("/stremio", h.handler.Router())
	h.server = httptest.NewServer(root)
	t.Cleanup(h.server.Close)
	return h
}

func TestManifest(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.server.URL + "/stremio/manifest.json")
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("manifest missing CORS header")
	}
	var m Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(m.Resources) != 1 || m.Resources[0] != "stream" {
		t.Errorf("resources = %v", m.Resources)
	}
	if len(m.Types) != 2 {
		t.Errorf("types = %v, want movie+series", m.Types)
	}
}

func TestManifestVersion(t *testing.T) {
	tests := map[string]string{
		"1.2.3":          "1.2.3",
		"v1.2.3":         "1.2.3",
		"1.2.3-rc.1":     "1.2.3-rc.1",
		"0.0.0-main.abc": "0.0.0-main.abc",
		"main":           fallbackVersion,
		"docker":         fallbackVersion,
		"dev":            fallbackVersion,
		"a1b2c3d":        fallbackVersion,
		"1.2":            fallbackVersion,
		"":               fallbackVersion,
	}
	for in, want := range tests {
		if got := manifestVersion(in); got != want {
			t.Errorf("manifestVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStreamDiscovery(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.server.URL + "/stremio/stream/movie/tt1375666.json")
	if err != nil {
		t.Fatalf("get stream: %v", err)
	}
	defer resp.Body.Close()

	var sr streamResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sr.Streams) != 2 {
		t.Fatalf("want 2 streams, got %d: %+v", len(sr.Streams), sr.Streams)
	}
	// Highest seeders first.
	if !strings.Contains(sr.Streams[0].Title, "42") {
		t.Errorf("expected top stream to have 42 seeders: %q", sr.Streams[0].Title)
	}
	if !strings.Contains(sr.Streams[0].URL, "/stremio/play/"+testHash) {
		t.Errorf("play URL = %q", sr.Streams[0].URL)
	}
}

func TestPlayRedirects(t *testing.T) {
	h := newHarness(t)

	// Don't follow the redirect — inspect the 302 target.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	playURL := h.server.URL + "/stremio/play/" + testHash + "?magnet=" + url.QueryEscape(testMagnet())
	resp, err := client.Get(playURL)
	if err != nil {
		t.Fatalf("play: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/dl/") {
		t.Errorf("redirect location = %q, want /dl/{token}", loc)
	}

	// The torrent was added to qBittorrent, stopped + sequential.
	var added bool
	for _, c := range h.fakeQB.Calls() {
		if strings.HasPrefix(c, "add magnet=") && strings.Contains(c, "seq=true") {
			added = true
		}
	}
	if !added {
		t.Errorf("magnet not added correctly: %v", h.fakeQB.Calls())
	}
}

func TestPlayRejectsMismatchedMagnet(t *testing.T) {
	h := newHarness(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	// Magnet hash differs from the path infohash.
	other := "magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff"
	resp, err := client.Get(h.server.URL + "/stremio/play/" + testHash + "?magnet=" + url.QueryEscape(other))
	if err != nil {
		t.Fatalf("play: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
