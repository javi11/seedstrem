package stremio

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
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

	// tt-sourced (IMDb) queries search Prowlarr by id token, split across
	// indexers by capability (see ttSearch). Anime ids have no
	// Prowlarr-recognized id token, so those resolve a title and search
	// by free text across whatever indexers are configured.
	var results []prowlarr.Result
	if q.IsAnime() {
		title, err := h.meta.AnimeTitle(ctx, q.Source, q.ID)
		if err != nil {
			h.logger.Warn("stremio: title resolution failed", "id", id, "error", err)
			writeJSON(w, http.StatusOK, empty)
			return
		}
		query, categories := buildAnimeSearch(q, title, s.Prowlarr)
		h.logger.Debug("stremio: prowlarr search",
			"query", query, "type", "search", "categories", categories, "indexers", len(s.Prowlarr.IndexerIDs))
		results, err = h.prowlarr(s).Search(ctx, query, "search", categories, s.Prowlarr.IndexerIDs)
		if err != nil {
			h.logger.Warn("stremio: prowlarr search failed", "query", query, "error", err)
			writeJSON(w, http.StatusOK, empty)
			return
		}
	} else {
		var err error
		results, err = h.ttSearch(ctx, q, s)
		if err != nil {
			h.logger.Warn("stremio: prowlarr search failed", "id", id, "error", err)
			writeJSON(w, http.StatusOK, empty)
			return
		}
	}

	raw := len(results)
	results = prowlarr.Sort(prowlarr.Filter(prowlarr.Dedup(results), s.Filters))
	if s.MaxResults > 0 && len(results) > s.MaxResults {
		results = results[:s.MaxResults]
	}
	h.logger.Debug("stremio: stream results", "id", id, "raw", raw, "returned", len(results))

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

// ttSearch searches Prowlarr for a tt-sourced (IMDb) query, splitting
// across indexers by capability: indexers whose Prowlarr definition
// declares ImdbId search support are searched by id token — precise, no
// title lookup needed. Indexers that don't are searched by free text as
// a fallback, which does need a resolved title, so Cinemeta is only
// queried when at least one such indexer is actually in scope. If the
// capability split itself can't be determined (Prowlarr unreachable, or
// the configured indexer ids don't match any known indexer), this falls
// back to a single id-token search scoped to whatever was configured —
// the pre-split behavior — rather than failing the whole request.
func (h *Handler) ttSearch(ctx context.Context, q meta.Query, s Settings) ([]prowlarr.Result, error) {
	pc := h.prowlarr(s)
	idQuery, searchType, categories := buildIDSearch(q, s.Prowlarr)

	indexers, err := h.cachedIndexers(ctx, pc)
	if err != nil {
		h.logger.Debug("stremio: indexer capability lookup failed, searching by id only", "error", err)
		return pc.Search(ctx, idQuery, searchType, categories, s.Prowlarr.IndexerIDs)
	}

	capable, incapable := splitByImdbSupport(indexers, s.Prowlarr.IndexerIDs, q.IsSeries())
	if len(capable) == 0 && len(incapable) == 0 {
		// Configured ids matched nothing we know about (e.g. stale
		// config) — same fallback as an unclassifiable capability lookup.
		return pc.Search(ctx, idQuery, searchType, categories, s.Prowlarr.IndexerIDs)
	}

	var results []prowlarr.Result
	if len(capable) > 0 {
		h.logger.Debug("stremio: prowlarr id search",
			"query", idQuery, "type", searchType, "indexers", len(capable))
		r, err := pc.Search(ctx, idQuery, searchType, categories, capable)
		if err != nil {
			return nil, fmt.Errorf("id search: %w", err)
		}
		results = append(results, r...)
	}

	if len(incapable) > 0 {
		title, year, err := h.resolveTitle(ctx, q)
		if err != nil {
			h.logger.Warn("stremio: fallback title resolution failed", "error", err)
			return results, nil
		}
		textQuery, textCategories := buildTextSearch(q, title, year, s.Prowlarr)
		h.logger.Debug("stremio: prowlarr text-fallback search",
			"query", textQuery, "indexers", len(incapable))
		r, err := pc.Search(ctx, textQuery, "search", textCategories, incapable)
		if err != nil {
			h.logger.Warn("stremio: prowlarr text-fallback search failed", "query", textQuery, "error", err)
			return results, nil
		}
		results = append(results, r...)
	}
	return results, nil
}

// splitByImdbSupport classifies enabled indexers within the configured
// scope (or every enabled indexer, when configured is empty) by whether
// their Prowlarr definition supports ImdbId search for this content type.
func splitByImdbSupport(indexers []prowlarr.IndexerInfo, configured []int, isSeries bool) (capable, incapable []int) {
	inScope := func(id int) bool {
		return len(configured) == 0 || slices.Contains(configured, id)
	}
	for _, ix := range indexers {
		if len(configured) == 0 && !ix.Enable {
			continue
		}
		if !inScope(ix.ID) {
			continue
		}
		supportsImdb := ix.SupportsMovieImdb()
		if isSeries {
			supportsImdb = ix.SupportsTvImdb()
		}
		if supportsImdb {
			capable = append(capable, ix.ID)
		} else {
			incapable = append(incapable, ix.ID)
		}
	}
	return capable, incapable
}

// resolveTitle returns the human title and (for movies) the release
// year, via Cinemeta. Only needed for the tt free-text fallback path.
func (h *Handler) resolveTitle(ctx context.Context, q meta.Query) (string, int, error) {
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

// buildAnimeSearch produces a free-text query for anime ids, which have
// no Prowlarr-recognized id token.
func buildAnimeSearch(q meta.Query, title string, p ProwlarrSettings) (query string, categories []int) {
	query = title
	if q.Episode > 0 {
		query = fmt.Sprintf("%s %02d", title, q.Episode)
	}
	return query, p.AnimeCategories
}

// buildIDSearch produces Prowlarr's id-token query syntax for a
// tt-sourced (IMDb) query — the same mechanism Radarr/Sonarr rely on —
// against "movie"/"tvsearch" search types.
func buildIDSearch(q meta.Query, p ProwlarrSettings) (query, searchType string, categories []int) {
	if q.IsSeries() {
		query = fmt.Sprintf("{ImdbId:%s}", q.ID)
		if q.Season > 0 {
			query += fmt.Sprintf("{Season:%02d}", q.Season)
		}
		if q.Episode > 0 {
			query += fmt.Sprintf("{Episode:%02d}", q.Episode)
		}
		return query, "tvsearch", p.TVCategories
	}
	return fmt.Sprintf("{ImdbId:%s}", q.ID), "movie", p.MovieCategories
}

// buildTextSearch produces a free-text fallback query (title/year/S-E)
// for tt-sourced queries scoped to indexers that don't support Prowlarr's
// ImdbId search parameter.
func buildTextSearch(q meta.Query, title string, year int, p ProwlarrSettings) (query string, categories []int) {
	if q.IsSeries() {
		if q.Season > 0 {
			return fmt.Sprintf("%s S%02dE%02d", title, q.Season, q.Episode), p.TVCategories
		}
		if q.Episode > 0 {
			return fmt.Sprintf("%s %02d", title, q.Episode), p.TVCategories
		}
		return title, p.TVCategories
	}
	if year > 0 {
		return fmt.Sprintf("%s %d", title, year), p.MovieCategories
	}
	return title, p.MovieCategories
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
