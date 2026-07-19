// Package downloader defines the backend-neutral download-client surface
// seedstrem programs against. Concrete backends (internal/qbit for
// qBittorrent, internal/deluge for Deluge) implement Client; everything
// else — torrent orchestration, streaming, syncing, cleanup — depends
// only on this package.
package downloader

import (
	"context"
	"errors"
)

// ErrTorrentNotFound is returned when the download client does not know
// the hash.
var ErrTorrentNotFound = errors.New("torrent not found in download client")

// ErrNotSupported is returned when the current backend cannot perform an
// operation (e.g. piece prioritization on qBittorrent, whose WebUI API
// has no per-piece primitive). Callers detect it with errors.Is and fall
// back. It is a dynamic capability signal on purpose: wrappers like
// Swappable forward it from whatever backend is live, which a static
// interface assertion could not express across hot-swaps.
var ErrNotSupported = errors.New("operation not supported by download client")

// Client is the download-client surface used by seedstrem.
type Client interface {
	AddMagnet(ctx context.Context, magnet string, opts AddOptions) error
	AddTorrentFile(ctx context.Context, raw []byte, opts AddOptions) error
	Torrents(ctx context.Context, hashes []string) ([]TorrentInfo, error)
	Torrent(ctx context.Context, hash string) (TorrentInfo, error)
	Files(ctx context.Context, hash string) ([]FileInfo, error)
	Properties(ctx context.Context, hash string) (Properties, error)
	PieceStates(ctx context.Context, hash string) ([]PieceState, error)
	SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error

	// SetSequentialDownload and SetFirstLastPiecePrio set the streaming
	// flags to an absolute state. Backends whose native API only offers a
	// blind toggle (qBittorrent) read the current state and toggle as
	// needed internally.
	SetSequentialDownload(ctx context.Context, hash string, on bool) error
	SetFirstLastPiecePrio(ctx context.Context, hash string, on bool) error

	Start(ctx context.Context, hash string) error
	Delete(ctx context.Context, hash string, deleteFiles bool) error

	// IncompleteFileHints describes where the backend keeps files that
	// are still downloading: an optional separate temp directory and an
	// optional extension appended to in-progress files. Zero values mean
	// the backend writes incomplete files in place under their final
	// names.
	IncompleteFileHints(ctx context.Context) (IncompleteHints, error)

	// PrioritizePieces asks the backend to fetch pieces [first, last] of
	// the torrent as soon as possible, ahead of the regular (sequential)
	// piece order. Backends without a per-piece primitive return
	// ErrNotSupported.
	PrioritizePieces(ctx context.Context, hash string, first, last int) error

	Version(ctx context.Context) (string, error)
}
