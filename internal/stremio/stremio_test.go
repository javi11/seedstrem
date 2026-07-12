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

	metaClient := meta.New(cinemeta.URL, "")

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

func TestBuildIDSearch(t *testing.T) {
	p := ProwlarrSettings{MovieCategories: []int{2000}, TVCategories: []int{5000}}

	movie := meta.Query{Source: "tt", ID: "tt1375666", Kind: meta.KindMovie}
	query, typ, cats := buildIDSearch(movie, p)
	if query != "{ImdbId:tt1375666}" {
		t.Errorf("movie query = %q, want id token", query)
	}
	if typ != "movie" {
		t.Errorf("movie type = %q, want movie", typ)
	}
	if len(cats) != 1 || cats[0] != 2000 {
		t.Errorf("movie categories = %v, want [2000]", cats)
	}

	series := meta.Query{Source: "tt", ID: "tt0944947", Kind: meta.KindSeries, Season: 1, Episode: 5}
	query, typ, cats = buildIDSearch(series, p)
	if query != "{ImdbId:tt0944947}{Season:01}{Episode:05}" {
		t.Errorf("series query = %q, want id+season+episode tokens", query)
	}
	if typ != "tvsearch" {
		t.Errorf("series type = %q, want tvsearch", typ)
	}
	if len(cats) != 1 || cats[0] != 5000 {
		t.Errorf("series categories = %v, want [5000]", cats)
	}

	seriesNoSE := meta.Query{Source: "tt", ID: "tt0944947", Kind: meta.KindSeries}
	if query, _, _ := buildIDSearch(seriesNoSE, p); query != "{ImdbId:tt0944947}" {
		t.Errorf("series without season/episode query = %q, want bare id token", query)
	}
}

func TestBuildTextSearch(t *testing.T) {
	p := ProwlarrSettings{MovieCategories: []int{2000}, TVCategories: []int{5000}}

	movie := meta.Query{Source: "tt", ID: "tt1375666", Kind: meta.KindMovie}
	if query, cats := buildTextSearch(movie, "The Matrix", 1999, p); query != "The Matrix 1999" || cats[0] != 2000 {
		t.Errorf("movie text search = %q, %v", query, cats)
	}

	series := meta.Query{Source: "tt", ID: "tt0944947", Kind: meta.KindSeries, Season: 1, Episode: 5}
	if query, cats := buildTextSearch(series, "Chernobyl", 0, p); query != "Chernobyl S01E05" || cats[0] != 5000 {
		t.Errorf("series text search = %q, %v", query, cats)
	}
}

func TestBuildAnimeSearch(t *testing.T) {
	p := ProwlarrSettings{AnimeCategories: []int{5070}}

	anime := meta.Query{Source: "kitsu", ID: "12", Kind: meta.KindMovie}
	if query, cats := buildAnimeSearch(anime, "Anime Movie", p); query != "Anime Movie" || cats[0] != 5070 {
		t.Errorf("anime search = %q, %v", query, cats)
	}

	animeEp := meta.Query{Source: "kitsu", ID: "44081", Kind: meta.KindSeries, Episode: 5}
	if query, _ := buildAnimeSearch(animeEp, "Anime Series", p); query != "Anime Series 05" {
		t.Errorf("anime series query = %q, want title + episode", query)
	}
}

func TestSplitByIDCapability(t *testing.T) {
	indexers := []prowlarr.IndexerInfo{
		{ID: 1, Enable: true, Capabilities: prowlarr.Capabilities{MovieSearchParams: []string{"Q", "ImdbId"}, TvSearchParams: []string{"Q"}}},
		{ID: 2, Enable: true, Capabilities: prowlarr.Capabilities{MovieSearchParams: []string{"Q"}, TvSearchParams: []string{"Q", "ImdbId"}}},
		{ID: 3, Enable: false, Capabilities: prowlarr.Capabilities{MovieSearchParams: []string{"Q", "ImdbId"}}},
		{ID: 4, Enable: true, Capabilities: prowlarr.Capabilities{MovieSearchParams: []string{"Q", "TmdbId"}, TvSearchParams: []string{"Q"}}},
	}

	// Empty configured scope = every enabled indexer, split by capability,
	// Imdb preferred over Tmdb over plain text.
	imdb, tmdb, text := splitByIDCapability(indexers, nil, false)
	if len(imdb) != 1 || imdb[0] != 1 {
		t.Errorf("movie imdb-capable = %v, want [1]", imdb)
	}
	if len(tmdb) != 1 || tmdb[0] != 4 {
		t.Errorf("movie tmdb-capable = %v, want [4]", tmdb)
	}
	if len(text) != 1 || text[0] != 2 {
		t.Errorf("movie text-only = %v, want [2]", text)
	}

	imdb, tmdb, text = splitByIDCapability(indexers, nil, true)
	if len(imdb) != 1 || imdb[0] != 2 {
		t.Errorf("tv imdb-capable = %v, want [2]", imdb)
	}
	if len(tmdb) != 0 {
		t.Errorf("tv tmdb-capable = %v, want none", tmdb)
	}
	if len(text) != 2 {
		t.Errorf("tv text-only = %v, want 2 (indexers 1 and 4)", text)
	}

	// A configured (disabled) indexer is still honored explicitly.
	imdb, _, _ = splitByIDCapability(indexers, []int{3}, false)
	if len(imdb) != 1 || imdb[0] != 3 {
		t.Errorf("explicit scope imdb-capable = %v, want [3]", imdb)
	}

	// Configured ids that match nothing known yield all-empty.
	imdb, tmdb, text = splitByIDCapability(indexers, []int{99}, false)
	if len(imdb) != 0 || len(tmdb) != 0 || len(text) != 0 {
		t.Errorf("unknown scope = imdb %v tmdb %v text %v, want all empty", imdb, tmdb, text)
	}
}

// TestStreamSplitsSearchByCapability verifies the end-to-end split: an
// ImdbId-capable indexer gets the id-token search, an incapable one gets
// a free-text fallback (requiring a title lookup), and both result sets
// are merged into the response.
func TestStreamSplitsSearchByCapability(t *testing.T) {
	const secondHash = "fedcba9876543210fedcba9876543210fedcba98"

	cinemeta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/meta/movie/tt1375666") {
			w.Write([]byte(`{"meta":{"name":"The Matrix","releaseInfo":"1999"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer cinemeta.Close()

	prow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/indexer":
			w.Write([]byte(`[
				{"id":1,"name":"Capable","protocol":"torrent","enable":true,
				 "capabilities":{"movieSearchParams":["Q","ImdbId"]}},
				{"id":2,"name":"Incapable","protocol":"torrent","enable":true,
				 "capabilities":{"movieSearchParams":["Q"]}}
			]`))
		case "/api/v1/search":
			q := r.URL.Query()
			switch {
			case q.Get("type") == "movie" && q["indexerIds"][0] == "1":
				w.Write([]byte(`[{"title":"ID Search Hit","magnetUrl":"` + testMagnet() + `","size":100,"seeders":10,"protocol":"torrent","indexer":"Capable"}]`))
			case q.Get("type") == "search" && q["indexerIds"][0] == "2":
				w.Write([]byte(`[{"title":"Text Search Hit","infoHash":"` + secondHash + `","size":200,"seeders":20,"protocol":"torrent","indexer":"Incapable"}]`))
			default:
				w.Write([]byte(`[]`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer prow.Close()

	metaClient := meta.New(cinemeta.URL, "")
	h := New(nil, metaClient, func() Settings {
		return Settings{
			Prowlarr:   ProwlarrSettings{URL: prow.URL, APIKey: "k", MovieCategories: []int{2000}},
			Addon:      AddonSettings{EnableMovies: true},
			MaxResults: 20,
		}
	}, "test", nil)

	root := chi.NewRouter()
	root.Mount("/stremio", h.Router())
	server := httptest.NewServer(root)
	defer server.Close()

	resp, err := http.Get(server.URL + "/stremio/stream/movie/tt1375666.json")
	if err != nil {
		t.Fatalf("get stream: %v", err)
	}
	defer resp.Body.Close()

	var sr streamResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sr.Streams) != 2 {
		t.Fatalf("want 2 streams (id search + text fallback merged), got %d: %+v", len(sr.Streams), sr.Streams)
	}
	var sawID, sawText bool
	for _, s := range sr.Streams {
		if strings.Contains(s.Title, "ID Search Hit") {
			sawID = true
		}
		if strings.Contains(s.Title, "Text Search Hit") {
			sawText = true
		}
	}
	if !sawID || !sawText {
		t.Errorf("expected both id-search and text-fallback results, got: %+v", sr.Streams)
	}
}

// TestStreamTmdbOnlyFallsBackToText verifies that a TmdbId-only indexer,
// when TMDb resolution isn't possible (no API key configured here),
// falls back to the free-text search rather than being dropped.
func TestStreamTmdbOnlyFallsBackToText(t *testing.T) {
	cinemeta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/meta/movie/tt1375666") {
			w.Write([]byte(`{"meta":{"name":"The Matrix","releaseInfo":"1999"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer cinemeta.Close()

	prow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/indexer":
			w.Write([]byte(`[
				{"id":1,"name":"TmdbOnly","protocol":"torrent","enable":true,
				 "capabilities":{"movieSearchParams":["Q","TmdbId"]}}
			]`))
		case "/api/v1/search":
			q := r.URL.Query()
			if q.Get("type") == "search" && q["indexerIds"][0] == "1" {
				w.Write([]byte(`[{"title":"Text Fallback Hit","magnetUrl":"` + testMagnet() + `","size":100,"seeders":10,"protocol":"torrent","indexer":"TmdbOnly"}]`))
				return
			}
			w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer prow.Close()

	// No TMDb API key configured: resolution errors, so the TmdbId-only
	// indexer should fall back to the text-search bucket, not be dropped.
	metaClient := meta.New(cinemeta.URL, "")
	h := New(nil, metaClient, func() Settings {
		return Settings{
			Prowlarr:   ProwlarrSettings{URL: prow.URL, APIKey: "k", MovieCategories: []int{2000}},
			Addon:      AddonSettings{EnableMovies: true},
			MaxResults: 20,
		}
	}, "test", nil)

	root := chi.NewRouter()
	root.Mount("/stremio", h.Router())
	server := httptest.NewServer(root)
	defer server.Close()

	resp, err := http.Get(server.URL + "/stremio/stream/movie/tt1375666.json")
	if err != nil {
		t.Fatalf("get stream: %v", err)
	}
	defer resp.Body.Close()

	var sr streamResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sr.Streams) != 1 || !strings.Contains(sr.Streams[0].Title, "Text Fallback Hit") {
		t.Fatalf("want text-fallback result for tmdb-only indexer, got: %+v", sr.Streams)
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
