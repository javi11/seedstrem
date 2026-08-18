package qbit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newFakeQbit stands up a minimal qBittorrent WebUI: cookie login plus
// /torrents/info, whose handler is supplied by the test.
func newFakeQbit(t *testing.T, info func(r *http.Request) string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
		w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/app/webapiVersion", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("2.9.3"))
	})
	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(info(r)))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestTorrentsByLabelFiltersByCategory(t *testing.T) {
	// The category must reach qBittorrent as a request parameter: asking
	// for everything and filtering locally would pull unrelated torrents
	// off a shared instance.
	gotCategory := make(chan string, 1)
	srv := newFakeQbit(t, func(r *http.Request) string {
		select {
		case gotCategory <- r.URL.Query().Get("category"):
		default:
		}
		return `[{"hash":"AAAA","name":"hand added","state":"stalledUP","added_on":1700000000}]`
	})
	c := New(srv.URL, "u", "p", "seedstrem")

	got, err := c.TorrentsByLabel(context.Background(), "seedstrem")
	if err != nil {
		t.Fatalf("by label: %v", err)
	}
	if cat := <-gotCategory; cat != "seedstrem" {
		t.Fatalf("category param = %q, want %q", cat, "seedstrem")
	}
	if len(got) != 1 || got[0].Hash != "AAAA" {
		t.Fatalf("got %+v, want one torrent AAAA", got)
	}
	if got[0].AddedAt.Unix() != 1700000000 {
		t.Fatalf("AddedAt = %v, want unix 1700000000", got[0].AddedAt)
	}
}

func TestTorrentsByLabelEmptyLabelReturnsNothing(t *testing.T) {
	// An empty category means "uncategorised" to qBittorrent, not "none",
	// so it must never reach the wire.
	srv := newFakeQbit(t, func(r *http.Request) string {
		t.Errorf("unexpected request for %s", r.URL)
		return `[]`
	})
	c := New(srv.URL, "u", "p", "")

	got, err := c.TorrentsByLabel(context.Background(), "")
	if err != nil {
		t.Fatalf("by label: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d torrents, want 0", len(got))
	}
}
