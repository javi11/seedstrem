package stremio

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/javib/seedstrem/internal/meta"
	"github.com/javib/seedstrem/internal/prowlarr"
)

// streamItem is one entry in a Stremio stream response.
type streamItem struct {
	Name          string         `json:"name"`
	Title         string         `json:"title"`
	URL           string         `json:"url"`
	BehaviorHints map[string]any `json:"behaviorHints,omitempty"`
}

type streamResponse struct {
	Streams []streamItem `json:"streams"`
}

// stream handles GET /stream/{type}/{id}.json — the discovery half. It
// always responds 200 with a (possibly empty) stream list; failures are
// logged and yield no streams so Stremio simply shows nothing.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := h.settings()
	typ := chi.URLParam(r, "type")
	id := strings.TrimSuffix(chi.URLParam(r, "id"), ".json")

	empty := streamResponse{Streams: []streamItem{}}

	h.logger.Debug("stremio: stream discovery request", "type", typ, "id", id)

	q, err := meta.ParseID(typ, id)
	if err != nil {
		h.logger.Debug("stremio: unparseable id", "type", typ, "id", id, "error", err)
		writeJSON(w, http.StatusOK, empty)
		return
	}
	if !h.contentEnabled(s.Addon, q) {
		h.logger.Debug("stremio: content type disabled", "id", id, "source", q.Source)
		writeJSON(w, http.StatusOK, empty)
		return
	}

	// tt-sourced (IMDb) queries search Prowlarr by id token — precise
	// enough that no title lookup is needed, and it skips a Cinemeta
	// round trip. Anime ids have no Prowlarr-recognized id token, so
	// those still resolve a title and search by free text.
	var title string
	if q.IsAnime() {
		var err error
		title, err = h.meta.AnimeTitle(ctx, q.Source, q.ID)
		if err != nil {
			h.logger.Warn("stremio: title resolution failed", "id", id, "error", err)
			writeJSON(w, http.StatusOK, empty)
			return
		}
	}

	query, searchType, categories := buildSearch(q, title, s.Prowlarr)
	h.logger.Debug("stremio: prowlarr search",
		"query", query, "type", searchType, "categories", categories, "indexers", len(s.Prowlarr.IndexerIDs))
	results, err := h.prowlarr(s).Search(ctx, query, searchType, categories, s.Prowlarr.IndexerIDs)
	if err != nil {
		h.logger.Warn("stremio: prowlarr search failed", "query", query, "error", err)
		writeJSON(w, http.StatusOK, empty)
		return
	}

	raw := len(results)
	results = prowlarr.Sort(prowlarr.Filter(prowlarr.Dedup(results), s.Filters))
	if s.MaxResults > 0 && len(results) > s.MaxResults {
		results = results[:s.MaxResults]
	}
	h.logger.Debug("stremio: stream results",
		"query", query, "raw", raw, "returned", len(results))

	items := make([]streamItem, 0, len(results))
	for _, res := range results {
		items = append(items, h.toStreamItem(s.ExternalURL, q, res))
	}
	writeJSON(w, http.StatusOK, streamResponse{Streams: items})
}

// contentEnabled reports whether the addon serves this query's content type.
func (h *Handler) contentEnabled(a AddonSettings, q meta.Query) bool {
	if q.IsAnime() {
		return a.EnableAnime
	}
	if q.IsSeries() {
		return a.EnableSeries
	}
	return a.EnableMovies
}

// buildSearch produces the Prowlarr query string, search type, and
// category list. tt-sourced (IMDb) queries use Prowlarr's id-token
// syntax against "movie"/"tvsearch" — the same mechanism Radarr/Sonarr
// rely on — so ID-capable indexers match precisely; title is unused in
// that case. Anime ids have no such token, so those fall back to a
// free-text "search" query built from the resolved title.
func buildSearch(q meta.Query, title string, p ProwlarrSettings) (query, searchType string, categories []int) {
	switch {
	case q.IsAnime():
		query := title
		if q.Episode > 0 {
			query = fmt.Sprintf("%s %02d", title, q.Episode)
		}
		return query, "search", p.AnimeCategories
	case q.IsSeries():
		query := fmt.Sprintf("{ImdbId:%s}", q.ID)
		if q.Season > 0 {
			query += fmt.Sprintf("{Season:%02d}", q.Season)
		}
		if q.Episode > 0 {
			query += fmt.Sprintf("{Episode:%02d}", q.Episode)
		}
		return query, "tvsearch", p.TVCategories
	default: // movie
		return fmt.Sprintf("{ImdbId:%s}", q.ID), "movie", p.MovieCategories
	}
}

// toStreamItem builds a Stremio stream pointing at the resolve-on-play
// endpoint. The magnet and selector are encoded statelessly in the URL.
func (h *Handler) toStreamItem(externalURL string, q meta.Query, res prowlarr.Result) streamItem {
	base := strings.TrimSuffix(externalURL, "/")
	v := url.Values{}
	v.Set("magnet", res.MagnetURL)
	if q.IsSeries() || q.IsAnime() {
		v.Set("series", "1")
		v.Set("s", strconv.Itoa(q.Season))
		v.Set("e", strconv.Itoa(q.Episode))
	}
	playURL := fmt.Sprintf("%s/stremio/play/%s?%s", base, res.InfoHash, v.Encode())

	title := fmt.Sprintf("%s\n👤 %d  💾 %s", res.Title, res.Seeders, humanSize(res.Size))
	if res.Indexer != "" {
		title += "  ⚙ " + res.Indexer
	}
	hints := map[string]any{}
	if q.IsSeries() || q.IsAnime() {
		hints["bingeGroup"] = "seedstrem-" + strings.ToLower(q.Source) + "-" + q.ID
	}
	return streamItem{
		Name:          "seedstrem",
		Title:         title,
		URL:           playURL,
		BehaviorHints: hints,
	}
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
