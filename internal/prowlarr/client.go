// Package prowlarr is a small client for Prowlarr's search API plus pure
// ranking/filtering helpers over the results.
package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/javib/seedstrem/internal/metainfo"
)

const defaultTimeout = 30 * time.Second

// Result is a normalized Prowlarr search release. Every Result returned
// by Search has both a non-empty InfoHash and a usable MagnetURL (a bare
// infohash is expanded into a magnet); releases with neither are dropped.
type Result struct {
	Title      string
	InfoHash   string // lowercase hex
	MagnetURL  string
	Size       int64
	Seeders    int
	Categories []int
	Indexer    string
}

// Client talks to a Prowlarr instance.
type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string
}

// New builds a Client with a default HTTP timeout.
func New(baseURL, apiKey string) *Client {
	return NewWithClient(baseURL, apiKey, &http.Client{Timeout: defaultTimeout})
}

// NewWithClient builds a Client with a caller-supplied http.Client
// (injectable for tests).
func NewWithClient(baseURL, apiKey string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{http: hc, baseURL: strings.TrimSuffix(baseURL, "/"), apiKey: apiKey}
}

// apiResult mirrors the fields we read from Prowlarr's /api/v1/search
// response. Categories are decoded leniently (see rawCategories).
type apiResult struct {
	Title      string          `json:"title"`
	InfoHash   string          `json:"infoHash"`
	MagnetURL  string          `json:"magnetUrl"`
	Size       int64           `json:"size"`
	Seeders    int             `json:"seeders"`
	Protocol   string          `json:"protocol"`
	Indexer    string          `json:"indexer"`
	Categories json.RawMessage `json:"categories"`
}

// Search queries Prowlarr for query across the given newznab categories.
// When indexerIDs is non-empty the search is scoped to those indexers;
// empty means search every enabled indexer. searchType selects Prowlarr's
// search mode ("search", "movie", or "tvsearch"); empty defaults to
// "search". For "movie"/"tvsearch", query is expected to carry Prowlarr's
// id tokens (e.g. "{ImdbId:tt1234567}{Season:01}{Episode:05}") so
// ID-capable indexers can match precisely instead of by free text — this
// mirrors how Radarr/Sonarr query Prowlarr.
func (c *Client) Search(ctx context.Context, query, searchType string, categories, indexerIDs []int) ([]Result, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("prowlarr: base URL not configured")
	}
	if searchType == "" {
		searchType = "search"
	}

	u, err := url.Parse(c.baseURL + "/api/v1/search")
	if err != nil {
		return nil, fmt.Errorf("prowlarr: bad base URL: %w", err)
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("type", searchType)
	for _, cat := range categories {
		q.Add("categories", strconv.Itoa(cat))
	}
	for _, id := range indexerIDs {
		q.Add("indexerIds", strconv.Itoa(id))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("prowlarr: build request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prowlarr: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr: search returned %d", resp.StatusCode)
	}

	var raw []apiResult
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("prowlarr: decode response: %w", err)
	}

	out := make([]Result, 0, len(raw))
	for _, r := range raw {
		if r.Protocol != "" && r.Protocol != "torrent" {
			continue // usenet or unknown — not resolvable via qBittorrent
		}
		res := normalize(r)
		if res.MagnetURL == "" {
			continue // no magnet and no infohash — cannot resolve-on-play
		}
		out = append(out, res)
	}
	return out, nil
}

// IndexerInfo is a Prowlarr indexer as reported by /api/v1/indexer.
type IndexerInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"` // "torrent" | "usenet"
	Enable   bool   `json:"enable"`
}

// Indexers lists the indexers configured in the Prowlarr instance so a
// caller can let the operator scope searches to a subset.
func (c *Client) Indexers(ctx context.Context) ([]IndexerInfo, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("prowlarr: base URL not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/indexer", nil)
	if err != nil {
		return nil, fmt.Errorf("prowlarr: build request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prowlarr: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr: indexer list returned %d", resp.StatusCode)
	}

	var out []IndexerInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("prowlarr: decode response: %w", err)
	}
	return out, nil
}

// normalize derives a lowercase infohash and a usable magnet URL. If the
// release carries only a magnet, the hash is parsed from it; if it
// carries only an infohash, a magnet is synthesized. Releases with
// neither yield an empty MagnetURL (dropped by the caller).
func normalize(r apiResult) Result {
	res := Result{
		Title:      r.Title,
		MagnetURL:  r.MagnetURL,
		Size:       r.Size,
		Seeders:    r.Seeders,
		Indexer:    r.Indexer,
		Categories: parseCategories(r.Categories),
	}
	res.InfoHash = strings.ToLower(strings.TrimSpace(r.InfoHash))

	if res.MagnetURL != "" {
		if hash, _, err := metainfo.FromMagnet(res.MagnetURL); err == nil {
			res.InfoHash = hash
		}
	}
	if res.MagnetURL == "" && res.InfoHash != "" {
		v := url.Values{}
		v.Set("dn", r.Title)
		res.MagnetURL = "magnet:?xt=urn:btih:" + res.InfoHash + "&" + v.Encode()
	}
	return res
}

// parseCategories decodes Prowlarr categories which may be either a list
// of ints (newznab ids) or a list of objects with an "id" field.
func parseCategories(raw json.RawMessage) []int {
	if len(raw) == 0 {
		return nil
	}
	var ints []int
	if err := json.Unmarshal(raw, &ints); err == nil {
		return ints
	}
	var objs []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &objs); err == nil {
		out := make([]int, 0, len(objs))
		for _, o := range objs {
			out = append(out, o.ID)
		}
		return out
	}
	return nil
}
