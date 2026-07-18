package torrents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/javib/seedstrem/internal/metainfo"
	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/store"
)

// metaPollInterval is how often WaitForMetadata re-checks qBittorrent for a
// resolved file list. The first check happens immediately.
const metaPollInterval = 1 * time.Second

// Settings is the live configuration slice the service needs, fetched per
// call so config hot-reload takes effect without restart.
type Settings struct {
	MetadataTimeout     time.Duration
	DeleteFilesOnRemove bool
}

// Service owns the add → wait → select → link mechanics against qBittorrent
// and the local store.
type Service struct {
	store    *store.Store
	dc       qbit.Client
	settings func() Settings
	logger   *slog.Logger

	// injectable for tests
	now   func() int64
	sleep func(ctx context.Context, d time.Duration) error
}

// New builds a Service.
func New(st *store.Store, dc qbit.Client, settings func() Settings, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    st,
		dc:       dc,
		settings: settings,
		logger:   logger,
		now:      func() int64 { return time.Now().Unix() },
		sleep:    sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// EnsureAdded adds a magnet to qBittorrent (stopped, sequential,
// first/last-piece priority) and persists the id↔hash mapping. It is
// idempotent on the infohash: a re-add returns the existing torrent.
func (s *Service) EnsureAdded(ctx context.Context, magnet string) (store.Torrent, error) {
	hash, name, err := metainfo.FromMagnet(magnet)
	if err != nil {
		return store.Torrent{}, fmt.Errorf("parse magnet: %w", err)
	}

	if existing, err := s.store.TorrentByHash(ctx, hash); err == nil {
		s.logger.Debug("torrents: reusing existing torrent", "hash", hash, "id", existing.ID)
		return existing, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Torrent{}, fmt.Errorf("lookup torrent by hash: %w", err)
	}

	opts := qbit.AddOptions{
		Stopped:            true,
		SequentialDownload: true,
		FirstLastPiecePrio: true,
	}
	s.logger.Debug("torrents: adding magnet to qbittorrent", "hash", hash, "name", name)
	if err := s.dc.AddMagnet(ctx, magnet, opts); err != nil {
		return store.Torrent{}, fmt.Errorf("add magnet to qbittorrent: %w", err)
	}

	id, err := NewID()
	if err != nil {
		return store.Torrent{}, fmt.Errorf("generate id: %w", err)
	}
	tor := store.Torrent{
		ID:      id,
		Hash:    hash,
		Name:    name,
		Phase:   store.PhaseAdded,
		AddedAt: s.now(),
		Magnet:  magnet,
	}
	if err := s.store.InsertTorrent(ctx, tor); err != nil {
		// A concurrent add of the same hash may have won the race (the
		// hash column is UNIQUE); fall back to the existing row.
		if existing, lookupErr := s.store.TorrentByHash(ctx, hash); lookupErr == nil {
			return existing, nil
		}
		return store.Torrent{}, fmt.Errorf("persist torrent: %w", err)
	}
	return tor, nil
}

// WaitForMetadata polls qBittorrent until it has resolved the torrent's file
// list (non-empty) or timeout elapses.
func (s *Service) WaitForMetadata(ctx context.Context, hash string, timeout time.Duration) ([]qbit.FileInfo, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		files, err := s.dc.Files(ctx, hash)
		if err != nil && !errors.Is(err, qbit.ErrTorrentNotFound) {
			return nil, fmt.Errorf("qbittorrent files: %w", err)
		}
		if len(files) > 0 {
			s.logger.Debug("torrents: metadata ready", "hash", hash, "files", len(files))
			return files, nil
		}
		if time.Now().After(deadline) {
			s.logger.Debug("torrents: metadata wait timed out", "hash", hash, "timeout", timeout)
			return nil, ErrMetadataTimeout
		}
		if err := s.sleep(ctx, metaPollInterval); err != nil {
			return nil, err
		}
	}
}

// ErrMetadataTimeout is returned when qBittorrent did not resolve the
// torrent's file list within the allotted time.
var ErrMetadataTimeout = errors.New("timed out waiting for torrent metadata")

// SelectAndLink marks fileIndex as wanted (others unwanted), starts the
// torrent, and mints a streaming link for that file. It is idempotent on
// (torrentID, fileIndex): a repeat call returns the existing link.
func (s *Service) SelectAndLink(ctx context.Context, tor store.Torrent, fileIndex int, files []qbit.FileInfo) (store.Link, error) {
	if existing, err := s.linkFor(ctx, tor.ID, fileIndex); err == nil {
		return existing, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Link{}, err
	}

	var selectedIdx, unselectedIdx []int
	for _, f := range files {
		if f.Index == fileIndex {
			selectedIdx = append(selectedIdx, f.Index)
		} else {
			unselectedIdx = append(unselectedIdx, f.Index)
		}
	}
	if len(selectedIdx) == 0 {
		return store.Link{}, fmt.Errorf("file index %d not in torrent", fileIndex)
	}

	if err := s.dc.SetFilePriority(ctx, tor.Hash, unselectedIdx, 0); err != nil {
		return store.Link{}, fmt.Errorf("deselect files: %w", err)
	}
	if err := s.dc.SetFilePriority(ctx, tor.Hash, selectedIdx, 1); err != nil {
		return store.Link{}, fmt.Errorf("select file: %w", err)
	}
	if err := s.dc.Start(ctx, tor.Hash); err != nil {
		return store.Link{}, fmt.Errorf("start torrent: %w", err)
	}

	token, err := NewLinkToken()
	if err != nil {
		return store.Link{}, fmt.Errorf("generate link token: %w", err)
	}
	var picked qbit.FileInfo
	for _, f := range files {
		if f.Index == fileIndex {
			picked = f
			break
		}
	}
	link := store.Link{
		Token:     token,
		TorrentID: tor.ID,
		FileIndex: fileIndex,
		Path:      picked.Name,
		Bytes:     picked.Size,
	}
	if err := s.store.InsertLinks(ctx, []store.Link{link}); err != nil {
		// Lost a race on the UNIQUE (torrent_id, file_index): return the
		// link the winner inserted.
		if existing, lookupErr := s.linkFor(ctx, tor.ID, fileIndex); lookupErr == nil {
			return existing, nil
		}
		return store.Link{}, fmt.Errorf("persist link: %w", err)
	}
	if tor.Phase != store.PhaseSelected {
		if err := s.store.SetTorrentPhase(ctx, tor.ID, store.PhaseSelected); err != nil {
			return store.Link{}, fmt.Errorf("set phase: %w", err)
		}
	}
	s.logger.Debug("torrents: file selected and linked",
		"hash", tor.Hash, "fileIndex", fileIndex, "path", picked.Name, "token", token)
	return link, nil
}

// linkFor returns the existing link for a torrent's file index, or
// store.ErrNotFound.
func (s *Service) linkFor(ctx context.Context, torrentID string, fileIndex int) (store.Link, error) {
	links, err := s.store.LinksByTorrent(ctx, torrentID)
	if err != nil {
		return store.Link{}, fmt.Errorf("lookup links: %w", err)
	}
	for _, l := range links {
		if l.FileIndex == fileIndex {
			return l, nil
		}
	}
	return store.Link{}, store.ErrNotFound
}

// Remove deletes a torrent from qBittorrent and the local store. A torrent
// already missing on either side is treated as already-removed, not an
// error.
func (s *Service) Remove(ctx context.Context, tor store.Torrent) error {
	deleteFiles := s.settings().DeleteFilesOnRemove
	if err := s.dc.Delete(ctx, tor.Hash, deleteFiles); err != nil && !errors.Is(err, qbit.ErrTorrentNotFound) {
		return fmt.Errorf("delete from qbittorrent: %w", err)
	}
	if err := s.store.DeleteTorrent(ctx, tor.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("delete from store: %w", err)
	}
	return nil
}

// Resolve is the end-to-end resolve-on-play flow: add the magnet, wait
// for metadata, pick the file matching sel, and mint a streaming link.
func (s *Service) Resolve(ctx context.Context, magnet string, sel Selector) (store.Link, error) {
	tor, err := s.EnsureAdded(ctx, magnet)
	if err != nil {
		return store.Link{}, err
	}
	files, err := s.WaitForMetadata(ctx, tor.Hash, s.settings().MetadataTimeout)
	if err != nil {
		return store.Link{}, err
	}
	idx, err := PickFile(files, sel)
	if err != nil {
		return store.Link{}, err
	}
	return s.SelectAndLink(ctx, tor, idx, files)
}
