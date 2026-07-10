// Package syncer reconciles the local store with qBittorrent in the
// background: torrents deleted out-of-band in qBittorrent get a sticky
// error, and display names are backfilled once metadata resolves.
package syncer

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/store"
)

// Syncer periodically reconciles store state against qBittorrent.
type Syncer struct {
	store    *store.Store
	qb       qbit.Client
	category func() string
	logger   *slog.Logger
	interval time.Duration
}

// New creates a Syncer. category is fetched per run so config changes
// apply without restart.
func New(st *store.Store, qb qbit.Client, category func() string, logger *slog.Logger, interval time.Duration) *Syncer {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Syncer{store: st, qb: qb, category: category, logger: logger, interval: interval}
}

// Run loops until ctx is cancelled.
func (s *Syncer) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("sync failed", "error", err)
			}
		}
	}
}

// Reconcile performs one reconciliation pass.
func (s *Syncer) Reconcile(ctx context.Context) error {
	qbTorrents, err := s.qb.Torrents(ctx, s.category())
	if err != nil {
		return err
	}
	live := make(map[string]qbit.TorrentInfo, len(qbTorrents))
	for _, t := range qbTorrents {
		live[strings.ToLower(t.Hash)] = t
	}

	stored, err := s.store.AllTorrents(ctx)
	if err != nil {
		return err
	}

	for _, tor := range stored {
		info, ok := live[tor.Hash]
		if !ok {
			if tor.Error == "" {
				s.logger.Info("torrent vanished from qbittorrent", "id", tor.ID, "hash", tor.Hash)
				if err := s.store.SetTorrentError(ctx, tor.ID, "removed from qBittorrent"); err != nil {
					s.logger.Warn("mark vanished torrent", "id", tor.ID, "error", err)
				}
			}
			continue
		}
		if tor.Error != "" {
			// Torrent is back (e.g. re-added manually): clear the error.
			if err := s.store.SetTorrentError(ctx, tor.ID, ""); err != nil {
				s.logger.Warn("clear torrent error", "id", tor.ID, "error", err)
			}
		}
		if tor.Name == "" && info.Name != "" {
			if err := s.store.SetTorrentName(ctx, tor.ID, info.Name); err != nil {
				s.logger.Warn("backfill torrent name", "id", tor.ID, "error", err)
			}
		}
	}
	return nil
}
