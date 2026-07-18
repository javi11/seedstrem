// Package cleanup periodically removes torrents that have seeded past
// their configured seed time.
package cleanup

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/javib/seedstrem/internal/playsession"
	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/store"
	"github.com/javib/seedstrem/internal/torrents"
)

// Settings is the live configuration slice the cleanup loop needs.
type Settings struct {
	// SeedTime is how long a completed torrent may seed before removal.
	// SeedTime <= 0 disables seed-time cleanup.
	SeedTime time.Duration
}

// Cleanup periodically sweeps stored torrents and removes finished ones
// that have seeded past Settings.SeedTime.
type Cleanup struct {
	store    *store.Store
	dc       qbit.Client
	svc      *torrents.Service
	sessions *playsession.Sessions
	settings func() Settings
	logger   *slog.Logger
	interval time.Duration
}

// New creates a Cleanup. sessions is consulted before removing a
// torrent so an actively-streamed torrent is never deleted out from
// under a viewer just because it has seeded past its time limit.
func New(st *store.Store, dc qbit.Client, svc *torrents.Service, sessions *playsession.Sessions, settings func() Settings, logger *slog.Logger, interval time.Duration) *Cleanup {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &Cleanup{store: st, dc: dc, svc: svc, sessions: sessions, settings: settings, logger: logger, interval: interval}
}

// Run loops until ctx is cancelled.
func (c *Cleanup) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Sweep(ctx); err != nil && ctx.Err() == nil {
				c.logger.Warn("cleanup sweep failed", "error", err)
			}
		}
	}
}

// Sweep performs one cleanup pass: torrents that have finished
// downloading and seeded past Settings.SeedTime are removed.
func (c *Cleanup) Sweep(ctx context.Context) error {
	seedTime := c.settings().SeedTime
	if seedTime <= 0 {
		return nil
	}

	stored, err := c.store.AllTorrents(ctx)
	if err != nil {
		return err
	}
	if len(stored) == 0 {
		return nil
	}

	hashes := make([]string, len(stored))
	for i, tor := range stored {
		hashes[i] = tor.Hash
	}
	dcTorrents, err := c.dc.Torrents(ctx, hashes)
	if err != nil {
		return err
	}
	live := make(map[string]qbit.TorrentInfo, len(dcTorrents))
	for _, t := range dcTorrents {
		live[strings.ToLower(t.Hash)] = t
	}

	for _, tor := range stored {
		info, ok := live[tor.Hash]
		if !ok || info.Progress < 1 || info.SeedingTime < seedTime {
			continue
		}

		done, ok := c.sessions.BeginRemoval(tor.Hash)
		if !ok {
			c.logger.Debug("cleanup: skipping torrent past seed time, currently being watched",
				"id", tor.ID, "hash", tor.Hash)
			continue
		}
		c.logger.Info("cleanup: removing torrent past seed time",
			"id", tor.ID, "hash", tor.Hash, "seedingTime", info.SeedingTime, "limit", seedTime)
		if err := c.svc.Remove(ctx, tor); err != nil {
			c.logger.Warn("cleanup: remove torrent", "id", tor.ID, "hash", tor.Hash, "error", err)
		}
		done()
	}
	return nil
}
