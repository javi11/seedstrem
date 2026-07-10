package stremio

import (
	"context"
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

	q, err := meta.ParseID(typ, id)
	if err != nil {
		h.logger.Debug("stremio: unparseable id", "type", typ, "id", id, "error", err)
		writeJSON(w, http.StatusOK, empty)
		return
	}
	if !h.contentEnabled(s.Addon, q) {
		writeJSON(w, http.StatusOK, empty)
		return
	}

	title, year, err := h.resolveTitle(ctx, q)
	if err != nil {
		h.logger.Warn("stremio: title resolution failed", "id", id, "error", err)
		writeJSON(w, http.StatusOK, empty)
		return
	}

	query, categories := buildSearch(q, title, year, s.Prowlarr)
	results, err := h.prowlarr(s).Search(ctx, query, categories)
	if err != nil {
		h.logger.Warn("stremio: prowlarr search failed", "query", query, "error", err)
		writeJSON(w, http.StatusOK, empty)
		return
	}

	results = prowlarr.Sort(prowlarr.Filter(prowlarr.Dedup(results), s.Filters))
	if s.MaxResults > 0 && len(results) > s.MaxResults {
		results = results[:s.MaxResults]
	}

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

// resolveTitle returns the human title and (for movies) the release year.
func (h *Handler) resolveTitle(ctx context.Context, q meta.Query) (string, int, error) {
	if q.Source == "tt" {
		typ := "movie"
		if q.IsSeries() {
			typ = "series"
		}
		info, err := h.meta.Meta(ctx, typ, q.ID)
		if err != nil {
			return "", 0, err
		}
		return info.Name, info.Year, nil
	}
	title, err := h.meta.AnimeTitle(ctx, q.Source, q.ID)
	if err != nil {
		return "", 0, err
	}
	return title, 0, nil
}

// buildSearch produces the Prowlarr query string and category list.
func buildSearch(q meta.Query, title string, year int, p ProwlarrSettings) (string, []int) {
	switch {
	case q.IsAnime():
		query := title
		if q.Episode > 0 {
			query = fmt.Sprintf("%s %02d", title, q.Episode)
		}
		return query, p.AnimeCategories
	case q.IsSeries():
		if q.Season > 0 {
			return fmt.Sprintf("%s S%02dE%02d", title, q.Season, q.Episode), p.TVCategories
		}
		if q.Episode > 0 {
			return fmt.Sprintf("%s %02d", title, q.Episode), p.TVCategories
		}
		return title, p.TVCategories
	default: // movie
		if year > 0 {
			return fmt.Sprintf("%s %d", title, year), p.MovieCategories
		}
		return title, p.MovieCategories
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
