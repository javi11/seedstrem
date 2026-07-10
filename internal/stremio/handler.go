// Package stremio implements seedstrem's Stremio addon: a stream-only
// manifest plus a stream handler (Prowlarr search) and a resolve-on-play
// handler that adds the chosen torrent to qBittorrent and redirects to
// the /dl streaming endpoint.
package stremio

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/javib/seedstrem/internal/meta"
	"github.com/javib/seedstrem/internal/prowlarr"
	"github.com/javib/seedstrem/internal/torrents"
)

// ProwlarrSettings is the live Prowlarr configuration.
type ProwlarrSettings struct {
	URL             string
	APIKey          string
	MovieCategories []int
	TVCategories    []int
	AnimeCategories []int
	IndexerIDs      []int
}

// AddonSettings toggles which content types the addon serves.
type AddonSettings struct {
	EnableMovies bool
	EnableSeries bool
	EnableAnime  bool
}

// Settings is the live configuration slice the handlers need, fetched per
// request so config hot-reload takes effect without restart.
type Settings struct {
	ExternalURL string
	Prowlarr    ProwlarrSettings
	Addon       AddonSettings
	Filters     prowlarr.Filters
	MaxResults  int
}

// Handler serves the Stremio addon endpoints.
type Handler struct {
	svc      *torrents.Service
	meta     *meta.Client
	settings func() Settings
	version  string
	logger   *slog.Logger
}

// New creates the addon handler. meta is shared (its cache persists);
// the Prowlarr client is built per request from live settings.
func New(svc *torrents.Service, m *meta.Client, settings func() Settings, version string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, meta: m, settings: settings, version: version, logger: logger}
}

// Router returns the chi router for mounting at /stremio.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(cors)
	r.Get("/manifest.json", h.manifest)
	r.Get("/stream/{type}/{id}", h.stream)
	r.Get("/play/{infohash}", h.play)
	r.Head("/play/{infohash}", h.play)
	return r
}

// cors adds the permissive CORS headers Stremio clients require.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) manifest(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, BuildManifest(h.version, h.settings().Addon))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) prowlarr(s Settings) *prowlarr.Client {
	return prowlarr.New(s.Prowlarr.URL, s.Prowlarr.APIKey)
}
