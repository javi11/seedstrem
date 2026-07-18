package deluge

import (
	"context"
	"sync/atomic"
)

// Swappable is a Client whose backing client can be replaced at
// runtime (when the admin UI changes Deluge connection settings).
type Swappable struct {
	current atomic.Pointer[Client]
}

// NewSwappable wraps an initial client.
func NewSwappable(c Client) *Swappable {
	s := &Swappable{}
	s.current.Store(&c)
	return s
}

// Swap replaces the backing client.
func (s *Swappable) Swap(c Client) {
	s.current.Store(&c)
}

func (s *Swappable) get() Client { return *s.current.Load() }

func (s *Swappable) AddMagnet(ctx context.Context, magnet string, opts AddOptions) error {
	return s.get().AddMagnet(ctx, magnet, opts)
}

func (s *Swappable) Torrents(ctx context.Context, hashes []string) ([]TorrentInfo, error) {
	return s.get().Torrents(ctx, hashes)
}

func (s *Swappable) Torrent(ctx context.Context, hash string) (TorrentInfo, error) {
	return s.get().Torrent(ctx, hash)
}

func (s *Swappable) Files(ctx context.Context, hash string) ([]FileInfo, error) {
	return s.get().Files(ctx, hash)
}

func (s *Swappable) Properties(ctx context.Context, hash string) (Properties, error) {
	return s.get().Properties(ctx, hash)
}

func (s *Swappable) PieceStates(ctx context.Context, hash string) ([]PieceState, error) {
	return s.get().PieceStates(ctx, hash)
}

func (s *Swappable) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	return s.get().SetFilePriority(ctx, hash, indices, priority)
}

func (s *Swappable) Start(ctx context.Context, hash string) error {
	return s.get().Start(ctx, hash)
}

func (s *Swappable) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	return s.get().Delete(ctx, hash, deleteFiles)
}

func (s *Swappable) Version(ctx context.Context) (string, error) {
	return s.get().Version(ctx)
}
