// Package stremio implements seedstrem's Stremio addon: a stream-only
// manifest plus a stream handler (Prowlarr search) and a resolve-on-play
// handler that adds the chosen torrent to qBittorrent and redirects to
// the /dl streaming endpoint.
package stremio

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/javib/seedstrem/internal/diskusage"
	"github.com/javib/seedstrem/internal/meta"
	"github.com/javib/seedstrem/internal/prowlarr"
	"github.com/javib/seedstrem/internal/torrents"
)

// indexerCacheTTL bounds how often the (id-search) capability split
// re-fetches Prowlarr's indexer list. A stream request otherwise pays
// an extra Prowlarr round trip on every single lookup.
const indexerCacheTTL = 5 * time.Minute

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

// DiskSettings configures the disk-usage gate that withholds new streams
// once the download disk is too full. A zero MaxUsagePercent or empty Path
// disables it.
type DiskSettings struct {
	// MaxUsagePercent is the used-disk percentage (0..100) at/above which
	// no new streams are offered. 0 disables the gate.
	MaxUsagePercent int
	// Path is the local directory whose filesystem usage is measured.
	Path string
}

// Settings is the live configuration slice the handlers need, fetched per
// request so config hot-reload takes effect without restart.
type Settings struct {
	ExternalURL string
	Prowlarr    ProwlarrSettings
	Addon       AddonSettings
	Filters     prowlarr.Filters
	MaxResults  int
	Disk        DiskSettings
}

// Handler serves the Stremio addon endpoints.
type Handler struct {
	svc      *torrents.Service
	meta     *meta.Client
	settings func() Settings
	version  string
	logger   *slog.Logger

	indexerCacheMu sync.Mutex
	indexerCache   []prowlarr.IndexerInfo
	indexerCacheAt time.Time

	torrentFiles *torrentFileCache

	// diskUsage reports (used, total) bytes for a local path. Injectable
	// for tests; defaults to diskusage.Stat.
	diskUsage func(path string) (used, total int64, err error)
}

// New creates the addon handler. meta is shared (its cache persists);
// the Prowlarr client is built per request from live settings.
func New(svc *torrents.Service, m *meta.Client, settings func() Settings, version string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, meta: m, settings: settings, version: version, logger: logger, torrentFiles: newTorrentFileCache(), diskUsage: diskusage.Stat}
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

// cachedIndexers returns the Prowlarr indexer list (with capabilities),
// refreshing at most once per indexerCacheTTL. Best-effort: a config
// change to the Prowlarr URL/key is picked up within the TTL window
// rather than instantly, which is an acceptable tradeoff against hitting
// Prowlarr on every stream request.
func (h *Handler) cachedIndexers(ctx context.Context, pc *prowlarr.Client) ([]prowlarr.IndexerInfo, error) {
	h.indexerCacheMu.Lock()
	if h.indexerCache != nil && time.Since(h.indexerCacheAt) < indexerCacheTTL {
		cached := h.indexerCache
		h.indexerCacheMu.Unlock()
		return cached, nil
	}
	h.indexerCacheMu.Unlock()

	indexers, err := pc.Indexers(ctx)
	if err != nil {
		return nil, err
	}

	h.indexerCacheMu.Lock()
	h.indexerCache = indexers
	h.indexerCacheAt = time.Now()
	h.indexerCacheMu.Unlock()
	return indexers, nil
}
