package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetaAndCache(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if !strings.HasPrefix(r.URL.Path, "/meta/movie/tt1375666") {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"meta":{"name":"The Matrix","releaseInfo":"1999"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	c.http = srv.Client()

	info, err := c.Meta(context.Background(), "movie", "tt1375666")
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if info.Name != "The Matrix" || info.Year != 1999 {
		t.Errorf("got %+v", info)
	}

	// Second call should be served from cache (no extra HTTP hit).
	if _, err := c.Meta(context.Background(), "movie", "tt1375666"); err != nil {
		t.Fatalf("meta 2: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 HTTP hit (cache), got %d", hits)
	}
}

func TestMetaYearFromRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"meta":{"name":"Breaking Bad","year":"2008–2013"}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "")
	c.http = srv.Client()
	info, err := c.Meta(context.Background(), "series", "tt0903747")
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if info.Year != 2008 {
		t.Errorf("year = %d, want 2008", info.Year)
	}
}

func TestAnimeTitleKitsu(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/anime/44081" {
			w.Write([]byte(`{"data":{"attributes":{"canonicalTitle":"Jujutsu Kaisen"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := New("", "")
	c.http = srv.Client()
	c.kitsuURL = srv.URL

	title, err := c.AnimeTitle(context.Background(), "kitsu", "44081")
	if err != nil {
		t.Fatalf("anime title: %v", err)
	}
	if title != "Jujutsu Kaisen" {
		t.Errorf("title = %q", title)
	}
}

func TestResolveTMDbID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/find/tt1375666" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("api_key") != "test-key" {
			t.Errorf("api_key = %q, want test-key", r.URL.Query().Get("api_key"))
		}
		if r.URL.Query().Get("external_source") != "imdb_id" {
			t.Errorf("external_source = %q, want imdb_id", r.URL.Query().Get("external_source"))
		}
		w.Write([]byte(`{"movie_results":[{"id":603}],"tv_results":[]}`))
	}))
	defer srv.Close()

	c := New("", "test-key")
	c.http = srv.Client()
	c.tmdbURL = srv.URL

	id, err := c.ResolveTMDbID(context.Background(), "movie", "tt1375666")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if id != 603 {
		t.Errorf("id = %d, want 603", id)
	}
}

func TestResolveTMDbIDNoAPIKey(t *testing.T) {
	c := New("", "")
	if _, err := c.ResolveTMDbID(context.Background(), "movie", "tt1375666"); err == nil {
		t.Fatal("expected error without an api key")
	}
}

func TestResolveTMDbIDNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"movie_results":[],"tv_results":[]}`))
	}))
	defer srv.Close()

	c := New("", "test-key")
	c.http = srv.Client()
	c.tmdbURL = srv.URL

	if _, err := c.ResolveTMDbID(context.Background(), "movie", "tt1375666"); err == nil {
		t.Fatal("expected error when tmdb has no match")
	}
}

func TestAnimeTitleMalMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mappings" && r.URL.Query().Get("filter[externalId]") == "20" {
			w.Write([]byte(`{"included":[{"type":"anime","attributes":{"canonicalTitle":"Naruto"}}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := New("", "")
	c.http = srv.Client()
	c.kitsuURL = srv.URL

	title, err := c.AnimeTitle(context.Background(), "mal", "20")
	if err != nil {
		t.Fatalf("anime title mal: %v", err)
	}
	if title != "Naruto" {
		t.Errorf("title = %q", title)
	}
}
