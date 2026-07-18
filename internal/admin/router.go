// Package admin serves the internal REST API used by the web UI:
// session login, configuration management, and status/torrent views.
package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/javib/seedstrem/internal/config"
	"github.com/javib/seedstrem/internal/deluge"
	"github.com/javib/seedstrem/internal/prowlarr"
	"github.com/javib/seedstrem/internal/store"
	"github.com/javib/seedstrem/internal/torrents"
)

const passwordMask = "••••••••"

// Handler serves the admin API.
type Handler struct {
	config  *config.Manager
	store   *store.Store
	dc      *deluge.Swappable
	logger  *slog.Logger
	version string
}

// New creates the admin handler. dc must be the swappable client so
// Deluge connection settings apply live.
func New(cm *config.Manager, st *store.Store, dc *deluge.Swappable, version string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{config: cm, store: st, dc: dc, logger: logger, version: version}
}

// Router returns the router to mount at /api.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	r.Post("/session", h.login)
	r.Group(func(r chi.Router) {
		r.Use(h.requireSession)
		r.Delete("/session", h.logout)
		r.Get("/session", h.sessionInfo)
		r.Get("/config", h.getConfig)
		r.Put("/config", h.putConfig)
		r.Post("/config/test-deluge", h.testDeluge)
		r.Post("/config/test-prowlarr", h.testProwlarr)
		r.Post("/config/prowlarr-indexers", h.prowlarrIndexers)
		r.Get("/status", h.status)
		r.Get("/torrents", h.torrents)
	})
	return r
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	want := h.config.Get().Server.AdminPassword
	if want == "" || subtle.ConstantTimeCompare([]byte(body.Password), []byte(want)) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	expiry := time.Now().Add(sessionLifetime)
	setSessionCookie(w, mintSession(want, expiry), expiry)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) logout(w http.ResponseWriter, _ *http.Request) {
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) sessionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

// configDTO is the JSON shape exchanged with the UI. The Deluge and
// admin passwords are masked on read; sending the mask back (or an
// empty string) keeps the stored value.
type configDTO struct {
	Server struct {
		Listen        string `json:"listen"`
		ExternalURL   string `json:"external_url"`
		AdminPassword string `json:"admin_password"`
	} `json:"server"`
	Deluge struct {
		Host     string `json:"host"`
		Port     uint   `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"deluge"`
	Prowlarr struct {
		URL             string `json:"url"`
		APIKey          string `json:"api_key"`
		MovieCategories []int  `json:"movie_categories"`
		TVCategories    []int  `json:"tv_categories"`
		AnimeCategories []int  `json:"anime_categories"`
		IndexerIDs      []int  `json:"indexer_ids"`
	} `json:"prowlarr"`
	Addon struct {
		EnableMovies bool `json:"enable_movies"`
		EnableSeries bool `json:"enable_series"`
		EnableAnime  bool `json:"enable_anime"`
	} `json:"addon"`
	Filters struct {
		MinSeeders int   `json:"min_seeders"`
		MinSizeMB  int64 `json:"min_size_mb"`
		MaxSizeMB  int64 `json:"max_size_mb"`
		MaxResults int   `json:"max_results"`
	} `json:"filters"`
	Meta struct {
		CinemetaURL            string `json:"cinemeta_url"`
		MetadataTimeoutSeconds int    `json:"metadata_timeout_seconds"`
		TMDbAPIKey             string `json:"tmdb_api_key"`
	} `json:"meta"`
	Paths struct {
		Mappings []config.Mapping `json:"mappings"`
	} `json:"paths"`
	Storage struct {
		DeleteFilesOnRemove bool `json:"delete_files_on_remove"`
	} `json:"storage"`
	Stream struct {
		WaitTimeoutSeconds int   `json:"wait_timeout_seconds"`
		ReadChunk          int64 `json:"read_chunk"`
	} `json:"stream"`
}

func toDTO(cfg config.Config) configDTO {
	var dto configDTO
	dto.Server.Listen = cfg.Server.Listen
	dto.Server.ExternalURL = cfg.Server.ExternalURL
	dto.Server.AdminPassword = passwordMask
	dto.Deluge.Host = cfg.Deluge.Host
	dto.Deluge.Port = cfg.Deluge.Port
	dto.Deluge.Username = cfg.Deluge.Username
	if cfg.Deluge.Password != "" {
		dto.Deluge.Password = passwordMask
	}
	dto.Prowlarr.URL = cfg.Prowlarr.URL
	if cfg.Prowlarr.APIKey != "" {
		dto.Prowlarr.APIKey = passwordMask
	}
	dto.Prowlarr.MovieCategories = cfg.Prowlarr.MovieCategories
	dto.Prowlarr.TVCategories = cfg.Prowlarr.TVCategories
	dto.Prowlarr.AnimeCategories = cfg.Prowlarr.AnimeCategories
	// Emit [] rather than null so clients can treat it as an array.
	dto.Prowlarr.IndexerIDs = cfg.Prowlarr.IndexerIDs
	if dto.Prowlarr.IndexerIDs == nil {
		dto.Prowlarr.IndexerIDs = []int{}
	}
	dto.Addon.EnableMovies = cfg.Addon.EnableMovies
	dto.Addon.EnableSeries = cfg.Addon.EnableSeries
	dto.Addon.EnableAnime = cfg.Addon.EnableAnime
	dto.Filters.MinSeeders = cfg.Filters.MinSeeders
	dto.Filters.MinSizeMB = cfg.Filters.MinSizeMB
	dto.Filters.MaxSizeMB = cfg.Filters.MaxSizeMB
	dto.Filters.MaxResults = cfg.Filters.MaxResults
	dto.Meta.CinemetaURL = cfg.Meta.CinemetaURL
	dto.Meta.MetadataTimeoutSeconds = int(cfg.Meta.MetadataTimeout / time.Second)
	if cfg.Meta.TMDbAPIKey != "" {
		dto.Meta.TMDbAPIKey = passwordMask
	}
	dto.Paths.Mappings = cfg.Paths.Mappings
	dto.Storage.DeleteFilesOnRemove = cfg.Storage.DeleteFilesOnRemove
	dto.Stream.WaitTimeoutSeconds = int(cfg.Stream.WaitTimeout / time.Second)
	dto.Stream.ReadChunk = cfg.Stream.ReadChunk
	return dto
}

// apply merges a DTO into an existing config, respecting mask/empty
// semantics for secrets.
func (dto configDTO) apply(cfg config.Config) config.Config {
	cfg.Server.Listen = dto.Server.Listen
	cfg.Server.ExternalURL = dto.Server.ExternalURL
	if pw := dto.Server.AdminPassword; pw != "" && pw != passwordMask {
		cfg.Server.AdminPassword = pw
	}
	cfg.Deluge.Host = dto.Deluge.Host
	cfg.Deluge.Port = dto.Deluge.Port
	cfg.Deluge.Username = dto.Deluge.Username
	// Empty or masked = keep the stored password.
	if pw := dto.Deluge.Password; pw != "" && pw != passwordMask {
		cfg.Deluge.Password = pw
	}
	cfg.Prowlarr.URL = dto.Prowlarr.URL
	// Empty or masked API key = keep the stored key.
	if k := dto.Prowlarr.APIKey; k != "" && k != passwordMask {
		cfg.Prowlarr.APIKey = k
	}
	cfg.Prowlarr.MovieCategories = dto.Prowlarr.MovieCategories
	cfg.Prowlarr.TVCategories = dto.Prowlarr.TVCategories
	cfg.Prowlarr.AnimeCategories = dto.Prowlarr.AnimeCategories
	cfg.Prowlarr.IndexerIDs = dto.Prowlarr.IndexerIDs
	cfg.Addon.EnableMovies = dto.Addon.EnableMovies
	cfg.Addon.EnableSeries = dto.Addon.EnableSeries
	cfg.Addon.EnableAnime = dto.Addon.EnableAnime
	cfg.Filters.MinSeeders = dto.Filters.MinSeeders
	cfg.Filters.MinSizeMB = dto.Filters.MinSizeMB
	cfg.Filters.MaxSizeMB = dto.Filters.MaxSizeMB
	cfg.Filters.MaxResults = dto.Filters.MaxResults
	cfg.Meta.CinemetaURL = dto.Meta.CinemetaURL
	if dto.Meta.MetadataTimeoutSeconds > 0 {
		cfg.Meta.MetadataTimeout = time.Duration(dto.Meta.MetadataTimeoutSeconds) * time.Second
	}
	// Empty or masked = keep the stored key.
	if k := dto.Meta.TMDbAPIKey; k != "" && k != passwordMask {
		cfg.Meta.TMDbAPIKey = k
	}
	cfg.Paths.Mappings = dto.Paths.Mappings
	cfg.Storage.DeleteFilesOnRemove = dto.Storage.DeleteFilesOnRemove
	if dto.Stream.WaitTimeoutSeconds > 0 {
		cfg.Stream.WaitTimeout = time.Duration(dto.Stream.WaitTimeoutSeconds) * time.Second
	}
	if dto.Stream.ReadChunk > 0 {
		cfg.Stream.ReadChunk = dto.Stream.ReadChunk
	}
	return cfg
}

func (h *Handler) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, toDTO(h.config.Get()))
}

func (h *Handler) putConfig(w http.ResponseWriter, r *http.Request) {
	var dto configDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	current := h.config.Get()
	next := dto.apply(current)

	if err := h.config.Update(next); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Hot-apply the Deluge client if its settings changed.
	if next.Deluge != current.Deluge {
		h.dc.Swap(deluge.New(next.Deluge.Host, next.Deluge.Port, next.Deluge.Username, next.Deluge.Password))
		h.logger.Info("deluge client reconfigured", "host", next.Deluge.Host, "port", next.Deluge.Port)
	}

	resp := map[string]any{"config": toDTO(next)}
	if next.Server.Listen != current.Server.Listen {
		resp["restart_required"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) testDeluge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host     string `json:"host"`
		Port     uint   `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Password == passwordMask {
		body.Password = h.config.Get().Deluge.Password
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	version, err := deluge.New(body.Host, body.Port, body.Username, body.Password).Version(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version})
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	cfg := h.config.Get()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	delugeStatus := map[string]any{"connected": false}
	if version, err := h.dc.Version(ctx); err == nil {
		delugeStatus = map[string]any{"connected": true, "version": version}
	} else {
		delugeStatus["error"] = err.Error()
	}

	counts := map[string]int{}
	if stored, err := h.store.AllTorrents(ctx); err == nil {
		live := h.liveByHash(ctx, stored)
		for _, tor := range stored {
			info, inDeluge := live[tor.Hash]
			status := torrents.DeriveStatus(tor.Phase, info.State, tor.Error != "" || !inDeluge, info.Size > 0, info.Progress)
			counts[status]++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":      h.version,
		"external_url": cfg.Server.ExternalURL,
		"manifest_url": strings.TrimSuffix(cfg.Server.ExternalURL, "/") + "/stremio/manifest.json",
		"deluge":       delugeStatus,
		"torrents":     counts,
	})
}

// liveByHash fetches live Deluge state for exactly the hashes seedstrem
// already tracks in the store (Deluge has no category/label concept to
// scope a listing by, unlike qBittorrent).
func (h *Handler) liveByHash(ctx context.Context, stored []store.Torrent) map[string]deluge.TorrentInfo {
	live := map[string]deluge.TorrentInfo{}
	if len(stored) == 0 {
		return live
	}
	hashes := make([]string, len(stored))
	for i, tor := range stored {
		hashes[i] = tor.Hash
	}
	if dcTorrents, err := h.dc.Torrents(ctx, hashes); err == nil {
		for _, t := range dcTorrents {
			live[strings.ToLower(t.Hash)] = t
		}
	}
	return live
}

// torrentItem is the UI torrent listing shape.
type torrentItem struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Hash     string     `json:"hash"`
	Status   string     `json:"status"`
	Progress float64    `json:"progress"`
	Speed    int64      `json:"speed"`
	Seeders  int64      `json:"seeders"`
	Size     int64      `json:"size"`
	AddedAt  int64      `json:"added_at"`
	Error    string     `json:"error,omitempty"`
	Links    []linkItem `json:"links"`
}

type linkItem struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	URL   string `json:"url"`
}

func (h *Handler) torrents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := h.config.Get()

	stored, _, err := h.store.ListTorrents(ctx, 500, 0)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list torrents")
		return
	}

	live := h.liveByHash(ctx, stored)

	base := strings.TrimSuffix(cfg.Server.ExternalURL, "/")
	items := make([]torrentItem, 0, len(stored))
	for _, tor := range stored {
		info, inDeluge := live[tor.Hash]
		links, err := h.store.LinksByTorrent(ctx, tor.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "load links")
			return
		}
		linkItems := make([]linkItem, 0, len(links))
		for _, l := range links {
			linkItems = append(linkItems, linkItem{Path: l.Path, Bytes: l.Bytes, URL: base + "/dl/" + l.Token})
		}
		name := tor.Name
		if info.Name != "" {
			name = info.Name
		}
		items = append(items, torrentItem{
			ID:       tor.ID,
			Name:     name,
			Hash:     tor.Hash,
			Status:   torrents.DeriveStatus(tor.Phase, info.State, tor.Error != "" || !inDeluge, info.Size > 0, info.Progress),
			Progress: info.Progress,
			Speed:    info.DlSpeed,
			Seeders:  info.NumSeeds,
			Size:     info.Size,
			AddedAt:  tor.AddedAt,
			Error:    tor.Error,
			Links:    linkItems,
		})
	}
	writeJSON(w, http.StatusOK, items)
}

// testProwlarr probes a Prowlarr instance with a trivial search so the UI
// can validate the URL + API key before saving.
func (h *Handler) testProwlarr(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL    string `json:"url"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.APIKey == passwordMask || body.APIKey == "" {
		body.APIKey = h.config.Get().Prowlarr.APIKey
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := prowlarr.New(body.URL, body.APIKey).Search(ctx, "test", "", nil, nil); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// prowlarrIndexers lists the enabled indexers of a Prowlarr instance so the
// UI can offer them as search targets. Uses the posted URL/API key, falling
// back to the stored key when masked/empty (same as testProwlarr).
func (h *Handler) prowlarrIndexers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL    string `json:"url"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.APIKey == passwordMask || body.APIKey == "" {
		body.APIKey = h.config.Get().Prowlarr.APIKey
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	indexers, err := prowlarr.New(body.URL, body.APIKey).Indexers(ctx)
	if err != nil {
		h.logger.Debug("admin: prowlarr indexer list failed", "url", body.URL, "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	type item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	out := make([]item, 0, len(indexers))
	for _, ix := range indexers {
		if !ix.Enable {
			continue
		}
		out = append(out, item{ID: ix.ID, Name: ix.Name})
	}
	h.logger.Debug("admin: prowlarr indexers listed", "total", len(indexers), "enabled", len(out))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "indexers": out})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
