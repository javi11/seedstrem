// Package fake provides an in-memory fake implementing deluge.Client
// directly. Unlike qBittorrent's HTTP WebUI API, Deluge's native RPC is
// a custom binary protocol, so faking the wire format would be far more
// work than it's worth — every consumer already depends on the
// deluge.Client Go interface, not a concrete transport, so faking at
// that boundary is sufficient and simpler.
package fake

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/javib/seedstrem/internal/deluge"
	"github.com/javib/seedstrem/internal/metainfo"
)

// File is one file of a fake torrent.
type File struct {
	Name     string
	Size     int64
	Progress float64
	Priority int
}

// Torrent is the mutable state of one fake torrent. Tests adjust fields
// directly via Server.Update.
type Torrent struct {
	Hash     string
	Name     string
	State    string
	Progress float64
	DlSpeed  int64
	NumSeeds int64
	SavePath string
	Files    []File

	PieceSize   int64
	PieceStates []int // deluge.PieceState values

	Stopped            bool
	SequentialDownload bool
	FirstLastPiecePrio bool
}

// Server is an in-memory fake deluge.Client. Tests construct one
// directly and pass it wherever a deluge.Client is expected — no
// separate adapter/URL is needed since it already satisfies the
// interface.
type Server struct {
	mu       sync.Mutex
	torrents map[string]*Torrent
	calls    []string
}

var _ deluge.Client = (*Server)(nil)

// New creates an empty fake.
func New() *Server {
	return &Server{torrents: map[string]*Torrent{}}
}

// Put inserts or replaces a torrent.
func (s *Server) Put(t *Torrent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.torrents[strings.ToLower(t.Hash)] = t
}

// Get returns a copy of the torrent, or nil if absent.
func (s *Server) Get(hash string) *Torrent {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return nil
	}
	cp := *t
	cp.Files = append([]File(nil), t.Files...)
	cp.PieceStates = append([]int(nil), t.PieceStates...)
	return &cp
}

// Update mutates a torrent under the lock.
func (s *Server) Update(hash string, fn func(*Torrent)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return false
	}
	fn(t)
	return true
}

// Calls returns a copy of the call log, in order.
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func (s *Server) record(format string, args ...any) {
	s.calls = append(s.calls, fmt.Sprintf(format, args...))
}

// --- deluge.Client ---

func (s *Server) AddMagnet(_ context.Context, magnet string, opts deluge.AddOptions) error {
	hash, name, err := metainfo.FromMagnet(magnet)
	if err != nil {
		return fmt.Errorf("fake: invalid magnet: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.record("add magnet=%s stopped=%v seq=%v flp=%v",
		hash, opts.Stopped, opts.SequentialDownload, opts.FirstLastPiecePrio)
	if _, exists := s.torrents[hash]; !exists {
		state := deluge.StateDownloading
		if opts.Stopped {
			state = deluge.StatePaused
		}
		s.torrents[hash] = &Torrent{
			Hash:               hash,
			Name:               name,
			State:              state,
			Stopped:            opts.Stopped,
			SequentialDownload: opts.SequentialDownload,
			FirstLastPiecePrio: opts.FirstLastPiecePrio,
		}
	}
	return nil
}

func (s *Server) Torrents(_ context.Context, hashes []string) ([]deluge.TorrentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, h := range hashes {
		want[strings.ToLower(h)] = true
	}
	out := make([]deluge.TorrentInfo, 0, len(hashes))
	for hash, t := range s.torrents {
		if !want[hash] {
			continue
		}
		out = append(out, toTorrentInfo(t))
	}
	return out, nil
}

func (s *Server) Torrent(_ context.Context, hash string) (deluge.TorrentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return deluge.TorrentInfo{}, deluge.ErrTorrentNotFound
	}
	return toTorrentInfo(t), nil
}

func toTorrentInfo(t *Torrent) deluge.TorrentInfo {
	return deluge.TorrentInfo{
		Hash:     t.Hash,
		Name:     t.Name,
		State:    t.State,
		Progress: t.Progress,
		DlSpeed:  t.DlSpeed,
		NumSeeds: t.NumSeeds,
		SavePath: t.SavePath,
	}
}

func (s *Server) Files(_ context.Context, hash string) ([]deluge.FileInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return nil, deluge.ErrTorrentNotFound
	}
	out := make([]deluge.FileInfo, len(t.Files))
	for i, f := range t.Files {
		out[i] = deluge.FileInfo{Index: i, Name: f.Name, Size: f.Size, Progress: f.Progress, Priority: f.Priority}
	}
	return out, nil
}

func (s *Server) Properties(_ context.Context, hash string) (deluge.Properties, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return deluge.Properties{}, deluge.ErrTorrentNotFound
	}
	return deluge.Properties{PieceSize: t.PieceSize, SavePath: t.SavePath}, nil
}

func (s *Server) PieceStates(_ context.Context, hash string) ([]deluge.PieceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return nil, deluge.ErrTorrentNotFound
	}
	out := make([]deluge.PieceState, len(t.PieceStates))
	for i, v := range t.PieceStates {
		out[i] = deluge.PieceState(v)
	}
	return out, nil
}

// Remove deletes a torrent entirely, as if the daemon forgot it.
func (s *Server) Remove(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.torrents, strings.ToLower(hash))
}

func (s *Server) SetFilePriority(_ context.Context, hash string, indices []int, priority int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return deluge.ErrTorrentNotFound
	}
	s.record("filePrio hash=%s indices=%v priority=%d", hash, indices, priority)
	for _, idx := range indices {
		if idx < 0 || idx >= len(t.Files) {
			continue
		}
		t.Files[idx].Priority = priority
	}
	return nil
}

func (s *Server) Start(_ context.Context, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.torrents[strings.ToLower(hash)]
	if !ok {
		return deluge.ErrTorrentNotFound
	}
	s.record("start hash=%s", hash)
	t.Stopped = false
	t.State = deluge.StateDownloading
	return nil
}

func (s *Server) Delete(_ context.Context, hash string, deleteFiles bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(hash)
	if _, ok := s.torrents[key]; !ok {
		return deluge.ErrTorrentNotFound
	}
	s.record("delete hash=%s deleteFiles=%v", hash, deleteFiles)
	delete(s.torrents, key)
	return nil
}

func (s *Server) Version(context.Context) (string, error) {
	return "2.1.1", nil
}
