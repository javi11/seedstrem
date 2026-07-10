package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultCinemetaURL = "https://v3-cinemeta.strem.io"
	defaultKitsuURL    = "https://kitsu.io/api/edge"
	metaCacheTTL       = 6 * time.Hour
	httpTimeout        = 15 * time.Second
)

// Info is the resolved title/year for a content id.
type Info struct {
	Name string
	Year int
}

// Client resolves titles from Cinemeta (IMDB) and Kitsu (anime), with a
// small TTL cache. Base URLs and the HTTP client are injectable for tests.
type Client struct {
	http        *http.Client
	cinemetaURL string
	kitsuURL    string

	mu    sync.Mutex
	cache map[string]cacheEntry
	ttl   time.Duration
	now   func() time.Time
}

type cacheEntry struct {
	info      Info
	expiresAt time.Time
}

// New builds a Client. An empty cinemetaURL falls back to the default.
func New(cinemetaURL string) *Client {
	if cinemetaURL == "" {
		cinemetaURL = defaultCinemetaURL
	}
	return &Client{
		http:        &http.Client{Timeout: httpTimeout},
		cinemetaURL: strings.TrimSuffix(cinemetaURL, "/"),
		kitsuURL:    defaultKitsuURL,
		cache:       map[string]cacheEntry{},
		ttl:         metaCacheTTL,
		now:         time.Now,
	}
}

func (c *Client) cacheGet(key string) (Info, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || c.now().After(e.expiresAt) {
		return Info{}, false
	}
	return e.info, true
}

func (c *Client) cachePut(key string, info Info) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{info: info, expiresAt: c.now().Add(c.ttl)}
}

var yearRe = regexp.MustCompile(`\d{4}`)

// Meta resolves title/year for an IMDB id via Cinemeta. typ is "movie"
// or "series".
func (c *Client) Meta(ctx context.Context, typ, imdbID string) (Info, error) {
	key := "cinemeta:" + typ + ":" + imdbID
	if info, ok := c.cacheGet(key); ok {
		return info, nil
	}

	url := fmt.Sprintf("%s/meta/%s/%s.json", c.cinemetaURL, typ, imdbID)
	var body struct {
		Meta struct {
			Name        string `json:"name"`
			Year        string `json:"year"`
			ReleaseInfo string `json:"releaseInfo"`
		} `json:"meta"`
	}
	if err := c.getJSON(ctx, url, &body); err != nil {
		return Info{}, err
	}
	if body.Meta.Name == "" {
		return Info{}, fmt.Errorf("cinemeta: no title for %s", imdbID)
	}
	info := Info{Name: body.Meta.Name, Year: parseYear(body.Meta.Year, body.Meta.ReleaseInfo)}
	c.cachePut(key, info)
	return info, nil
}

func parseYear(fields ...string) int {
	for _, f := range fields {
		if m := yearRe.FindString(f); m != "" {
			y, _ := strconv.Atoi(m)
			return y
		}
	}
	return 0
}

func (c *Client) getJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("meta: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("meta: request %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("meta: %s returned %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("meta: decode %s: %w", url, err)
	}
	return nil
}
