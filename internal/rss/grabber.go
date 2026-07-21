// Package rss implements a background grabber that periodically pulls the
// just-released items from the configured Prowlarr indexers and
// auto-downloads a filtered subset. Its purpose is twofold: to build
// seeding ratio (fresh, well-seeded, optionally freeleech releases), and
// to warm a cache so that when Stremio later requests that content the
// torrent is already downloading/complete and surfaces first as "ready".
//
// It reuses the existing on-demand machinery: Prowlarr's search endpoint
// (an empty query returns each indexer's recent releases), the
// prowlarr.Dedup/Filter/Sort ranking helpers, and torrents.Service to add
// downloads. Disk is bounded by the same disk-usage gate the stream path
// uses and by the seed-time cleanup sweep, which removes grabbed torrents
// once they have seeded long enough.
package rss

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/javib/seedstrem/internal/diskusage"
	"github.com/javib/seedstrem/internal/prowlarr"
	"github.com/javib/seedstrem/internal/store"
	"github.com/javib/seedstrem/internal/torrents"
)

// Settings is the live configuration slice the grabber needs, fetched per
// poll so config hot-reload takes effect without a restart.
type Settings struct {
	Enabled        bool
	ProwlarrURL    string
	ProwlarrAPIKey string
	// Categories are the newznab category ids to poll, already combined
	// for whichever content types the addon serves.
	Categories []int
	// IndexerIDs scopes the poll to specific Prowlarr indexers; empty
	// means every enabled indexer.
	IndexerIDs []int
	// Filters constrains releases by seeders and size. Only MinSeeders is
	// inherited from the global filters.* (a ratio-safety floor); the size
	// bounds are populated from the independent rss.filters.* section.
	Filters prowlarr.Filters
	// FreeleechOnly restricts grabs to freeleech releases (ratio-safe).
	FreeleechOnly bool
	// IncludeKeywords keeps only releases whose title contains at least one
	// of these (case-insensitive substring). Empty allows all titles.
	IncludeKeywords []string
	// ExcludeKeywords drops releases whose title contains any of these
	// (case-insensitive substring). Exclude takes precedence over include.
	ExcludeKeywords []string
	// MaxGrabsPerCycle caps additions per poll. 0 disables grabbing.
	MaxGrabsPerCycle int
	// DiskPath and MaxDiskUsagePercent gate grabbing on free disk space,
	// mirroring the stream handler's disk gate. 0 percent disables it.
	DiskPath            string
	MaxDiskUsagePercent int
}

// searcher is the slice of *prowlarr.Client the grabber depends on.
type searcher interface {
	Search(ctx context.Context, query, searchType string, categories, indexerIDs []int) ([]prowlarr.Result, error)
}

// downloadAdder is the slice of *torrents.Service used to add a grabbed
// release. An empty Selector means no file is singled out, so the whole
// torrent downloads — exactly what seeding wants.
type downloadAdder interface {
	EnsureAdded(ctx context.Context, magnet string, torrentFile []byte, sel torrents.Selector) (store.Torrent, error)
}

// ownedLookup reports which of the given infohashes are already stored (as
// a map keyed by lowercase infohash), so the grabber skips re-adding them.
// Batched to avoid a per-candidate query each poll cycle.
type ownedLookup interface {
	TorrentsByHashes(ctx context.Context, hashes []string) (map[string]store.Torrent, error)
}

// Grabber periodically polls Prowlarr and grabs recent releases.
type Grabber struct {
	store    ownedLookup
	svc      downloadAdder
	settings func() Settings
	logger   *slog.Logger
	interval time.Duration

	// injectable for tests
	newSearcher func(url, apiKey string) searcher
	diskUsage   func(path string) (used, total int64, err error)
}

// New builds a Grabber. interval is fixed at startup (mirroring syncer and
// cleanup); changing rss.interval takes effect on the next restart.
func New(st ownedLookup, svc downloadAdder, settings func() Settings, logger *slog.Logger, interval time.Duration) *Grabber {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &Grabber{
		store:       st,
		svc:         svc,
		settings:    settings,
		logger:      logger,
		interval:    interval,
		newSearcher: func(url, apiKey string) searcher { return prowlarr.New(url, apiKey) },
		diskUsage:   diskusage.Stat,
	}
}

// Run loops until ctx is cancelled, polling once per interval.
func (g *Grabber) Run(ctx context.Context) {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := g.Poll(ctx); err != nil && ctx.Err() == nil {
				g.logger.Warn("rss poll failed", "error", err)
			}
		}
	}
}

// Poll performs one grab cycle: fetch recent releases, filter/rank them,
// and add the selected subset to the download client.
func (g *Grabber) Poll(ctx context.Context) error {
	s := g.settings()
	if !s.Enabled || s.MaxGrabsPerCycle <= 0 {
		return nil
	}
	if s.ProwlarrURL == "" {
		g.logger.Debug("rss: prowlarr not configured, skipping poll")
		return nil
	}
	if len(s.Categories) == 0 {
		g.logger.Debug("rss: no categories enabled, skipping poll")
		return nil
	}

	// An empty query triggers Prowlarr's recent-releases (RSS) behavior for
	// each indexer, reusing all the normalization/magnet-synthesis in the
	// existing client.
	results, err := g.newSearcher(s.ProwlarrURL, s.ProwlarrAPIKey).
		Search(ctx, "", "search", s.Categories, s.IndexerIDs)
	if err != nil {
		return fmt.Errorf("rss: fetch recent releases: %w", err)
	}

	var used, total int64
	if s.MaxDiskUsagePercent > 0 {
		if s.DiskPath == "" {
			// A configured threshold with no measurable path can't gate
			// anything — surface it so an unbounded auto-downloader isn't
			// running with a silently-disabled disk gate.
			g.logger.Warn("rss: max_disk_usage_percent set but no local path mapping; disk gate disabled")
		} else {
			u, tot, derr := g.diskUsage(s.DiskPath)
			if derr != nil {
				// Fail closed: unlike serving an already-owned stream, adding
				// NEW downloads when free space can't be measured risks
				// filling the disk. Skip this cycle rather than grab blind.
				g.logger.Warn("rss: disk usage check failed; skipping grab cycle", "path", s.DiskPath, "error", derr)
				return nil
			}
			used, total = u, tot
		}
	}

	// One batched lookup of which candidates we already have, rather than a
	// per-candidate query. Fail closed: a store error means we can't tell
	// what's owned, so skip the cycle rather than risk re-downloading.
	hashes := make([]string, 0, len(results))
	for _, r := range results {
		if r.InfoHash != "" {
			hashes = append(hashes, r.InfoHash)
		}
	}
	owned, err := g.store.TorrentsByHashes(ctx, hashes)
	if err != nil {
		g.logger.Warn("rss: owned-lookup failed; skipping grab cycle", "error", err)
		return nil
	}
	have := func(hash string) bool {
		_, ok := owned[strings.ToLower(hash)]
		return ok
	}

	grabs := selectGrabs(results, s, used, total, have)
	if len(grabs) == 0 {
		g.logger.Debug("rss: nothing to grab this cycle", "candidates", len(results))
		return nil
	}

	added := 0
	for _, r := range grabs {
		if _, err := g.svc.EnsureAdded(ctx, r.MagnetURL, r.TorrentFile, torrents.Selector{}); err != nil {
			g.logger.Warn("rss: grab failed", "title", r.Title, "hash", r.InfoHash, "error", err)
			continue
		}
		added++
		g.logger.Info("rss: grabbed release",
			"title", r.Title, "hash", r.InfoHash, "size", r.Size,
			"seeders", r.Seeders, "freeleech", r.Freeleech, "indexer", r.Indexer)
	}
	g.logger.Debug("rss: grab cycle complete", "candidates", len(results), "selected", len(grabs), "added", added)
	return nil
}

// selectGrabs is the pure decision core: given recent releases, live
// settings, and current disk usage, it returns the releases to grab this
// cycle. It dedups (by infohash, then by release title so the same release
// mirrored across indexers is grabbed once), applies the seeder/size
// filters, optionally keeps only freeleech, ranks (freeleech → seeders →
// size), then walks the ranked list skipping already-owned releases and any
// that would push disk usage past the threshold, stopping at
// MaxGrabsPerCycle. have reports whether a release's infohash is already
// downloaded; a nil have treats nothing as owned.
func selectGrabs(results []prowlarr.Result, s Settings, used, total int64, have func(hash string) bool) []prowlarr.Result {
	if s.MaxGrabsPerCycle <= 0 {
		return nil
	}

	ranked := prowlarr.Filter(prowlarr.Dedup(results), s.Filters)
	ranked = filterByTitle(ranked, s.IncludeKeywords, s.ExcludeKeywords)
	if s.FreeleechOnly {
		fl := make([]prowlarr.Result, 0, len(ranked))
		for _, r := range ranked {
			if r.Freeleech {
				fl = append(fl, r)
			}
		}
		ranked = fl
	}
	ranked = prowlarr.Sort(ranked)
	// Collapse the same release listed on multiple indexers. Prowlarr's
	// infohash Dedup misses these because each tracker repacks the .torrent
	// (different trackers/comment ⇒ different infohash) for the same scene
	// release. Ranked is already best-first, so keeping the first occurrence
	// of each normalized title keeps the best-seeded/freeleech copy.
	ranked = dedupeByReleaseTitle(ranked)

	// Disk limit: bytes we must stay under. limit <= 0 means "no gate".
	var limit int64
	if s.MaxDiskUsagePercent > 0 && total > 0 {
		limit = total * int64(s.MaxDiskUsagePercent) / 100
		if used >= limit {
			return nil // disk already at/over the threshold: grab nothing
		}
	}

	out := make([]prowlarr.Result, 0, s.MaxGrabsPerCycle)
	projected := used
	for _, r := range ranked {
		if len(out) >= s.MaxGrabsPerCycle {
			break
		}
		if r.InfoHash == "" {
			continue // nothing to dedup/track on
		}
		if have != nil && have(r.InfoHash) {
			continue // already downloaded
		}
		if limit > 0 && projected+r.Size > limit {
			continue // would push disk usage past the threshold
		}
		out = append(out, r)
		projected += r.Size
	}
	return out
}

// filterByTitle applies the RSS-specific keyword gates to release titles,
// matched case-insensitively as substrings. A release is dropped if its
// title contains any exclude keyword (exclude wins), then kept only if it
// contains at least one include keyword. Empty lists disable their gate:
// no includes means "allow all", no excludes means "drop none".
func filterByTitle(results []prowlarr.Result, include, exclude []string) []prowlarr.Result {
	// Normalize each keyword list once (lower-case, trimmed, blanks dropped)
	// rather than per title comparison.
	inc := normalizeKeywords(include)
	exc := normalizeKeywords(exclude)
	if len(inc) == 0 && len(exc) == 0 {
		return results
	}
	out := make([]prowlarr.Result, 0, len(results))
	for _, r := range results {
		lower := strings.ToLower(r.Title)
		if containsAny(lower, exc) {
			continue
		}
		if len(inc) > 0 && !containsAny(lower, inc) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// normalizeKeywords lower-cases and trims each keyword, dropping blanks so a
// stray empty entry can't later match every title.
func normalizeKeywords(keywords []string) []string {
	out := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		if kw = strings.TrimSpace(strings.ToLower(kw)); kw != "" {
			out = append(out, kw)
		}
	}
	return out
}

// containsAny reports whether lowered contains any of the (already
// normalized) keywords.
func containsAny(lowered string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(lowered, kw) {
			return true
		}
	}
	return false
}

// dedupeByReleaseTitle keeps only the first result for each normalized
// release title, preserving input order. Callers pass an already-ranked
// slice so the survivor is the best-ranked copy. Results whose title
// normalizes to empty are always kept (nothing reliable to group on).
func dedupeByReleaseTitle(results []prowlarr.Result) []prowlarr.Result {
	seen := make(map[string]struct{}, len(results))
	out := make([]prowlarr.Result, 0, len(results))
	for _, r := range results {
		key := normalizeReleaseTitle(r.Title)
		if key != "" {
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, r)
	}
	return out
}

// normalizeReleaseTitle reduces a release name to a comparison key by
// lower-casing and keeping only alphanumerics. Scene/P2P release names are
// otherwise byte-identical across trackers, so this collapses separator and
// case differences ("The.Matrix.1999.1080p" vs "the matrix 1999 1080p")
// while keeping genuinely different releases (resolution, group, repack)
// distinct.
func normalizeReleaseTitle(title string) string {
	var b strings.Builder
	b.Grow(len(title))
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
