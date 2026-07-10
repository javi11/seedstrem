package prowlarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const hexHash = "0123456789abcdef0123456789abcdef01234567"

func TestSearch(t *testing.T) {
	var gotKey, gotQuery string
	var gotCategories []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		gotQuery = r.URL.Query().Get("query")
		gotCategories = r.URL.Query()["categories"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"title":"Has Magnet 1080p","magnetUrl":"magnet:?xt=urn:btih:` + hexHash + `&dn=x","size":100,"seeders":10,"protocol":"torrent","indexer":"idx","categories":[2040]},
			{"title":"Infohash Only 720p","infoHash":"` + strings.ToUpper(hexHash) + `","size":50,"seeders":5,"protocol":"torrent","categories":[{"id":5040}]},
			{"title":"Torrent File Only","downloadUrl":"http://x/file.torrent","size":30,"seeders":3,"protocol":"torrent"},
			{"title":"Usenet Release","infoHash":"deadbeef","protocol":"usenet","size":40,"seeders":99}
		]`))
	}))
	defer srv.Close()

	c := NewWithClient(srv.URL, "my-key", srv.Client())
	results, err := c.Search(context.Background(), "the matrix 1999", []int{2000, 5000})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if gotKey != "my-key" {
		t.Errorf("X-Api-Key = %q, want my-key", gotKey)
	}
	if gotQuery != "the matrix 1999" {
		t.Errorf("query = %q", gotQuery)
	}
	if len(gotCategories) != 2 {
		t.Errorf("categories params = %v, want 2", gotCategories)
	}

	// Kept: magnet result + infohash-only (synthesized magnet). Dropped:
	// torrent-file-only (no magnet/infohash) and usenet.
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(results), results)
	}

	if results[0].InfoHash != hexHash {
		t.Errorf("magnet result infohash = %q, want %q", results[0].InfoHash, hexHash)
	}
	if len(results[0].Categories) != 1 || results[0].Categories[0] != 2040 {
		t.Errorf("int categories not parsed: %v", results[0].Categories)
	}

	second := results[1]
	if second.InfoHash != hexHash {
		t.Errorf("infohash-only result should lowercase hash, got %q", second.InfoHash)
	}
	if !strings.HasPrefix(second.MagnetURL, "magnet:?xt=urn:btih:"+hexHash) {
		t.Errorf("infohash-only result should synthesize magnet, got %q", second.MagnetURL)
	}
	if len(second.Categories) != 1 || second.Categories[0] != 5040 {
		t.Errorf("object categories not parsed: %v", second.Categories)
	}
}

func TestSearchNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewWithClient(srv.URL, "bad", srv.Client())
	if _, err := c.Search(context.Background(), "q", nil); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestSearchNoBaseURL(t *testing.T) {
	c := New("", "key")
	if _, err := c.Search(context.Background(), "q", nil); err == nil {
		t.Fatal("expected error when base URL unset")
	}
}
