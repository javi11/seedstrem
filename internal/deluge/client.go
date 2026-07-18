// Package deluge wraps the Deluge daemon's native RPC API behind a
// narrow interface tailored to seedstrem's needs. It targets Deluge 2.x
// daemons (V2 RPC) via github.com/autobrr/go-deluge — vendored locally
// with two added RPC calls upstream is missing; see
// third_party/go-deluge/FORK_NOTES.md.
package deluge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	libdeluge "github.com/autobrr/go-deluge"
)

// Client is the Deluge surface used by seedstrem.
type Client interface {
	AddMagnet(ctx context.Context, magnet string, opts AddOptions) error
	Torrents(ctx context.Context, hashes []string) ([]TorrentInfo, error)
	Torrent(ctx context.Context, hash string) (TorrentInfo, error)
	Files(ctx context.Context, hash string) ([]FileInfo, error)
	Properties(ctx context.Context, hash string) (Properties, error)
	PieceStates(ctx context.Context, hash string) ([]PieceState, error)
	SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error
	Start(ctx context.Context, hash string) error
	Delete(ctx context.Context, hash string, deleteFiles bool) error
	Version(ctx context.Context) (string, error)
}

// ErrTorrentNotFound is returned when Deluge does not know the hash.
var ErrTorrentNotFound = fmt.Errorf("torrent not found in deluge")

type client struct {
	mu        sync.Mutex
	settings  libdeluge.Settings
	dc        *libdeluge.ClientV2
	connected bool
}

// New creates a Client for the Deluge daemon RPC at host:port. The
// connection is established lazily on first use, so New itself cannot
// fail — matching the rest of seedstrem's hot-swappable client pattern.
func New(host string, port uint, username, password string) Client {
	return &client{settings: libdeluge.Settings{
		Hostname: host,
		Port:     port,
		Login:    username,
		Password: password,
	}}
}

// ensureConnected dials and logs in on first use, or reuses the existing
// connection. A failed RPC call marks the connection stale (see
// markDisconnected) so the next call reconnects rather than retrying
// forever against a dead socket.
func (c *client) ensureConnected(ctx context.Context) (*libdeluge.ClientV2, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dc == nil {
		c.dc = libdeluge.NewV2(c.settings)
	}
	if c.connected {
		return c.dc, nil
	}
	if err := c.dc.Connect(ctx); err != nil {
		return nil, fmt.Errorf("deluge connect: %w", err)
	}
	c.connected = true
	return c.dc, nil
}

func (c *client) markDisconnected() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

// isNotFound reports whether err is Deluge's InvalidTorrentError (raised
// by the daemon for an unrecognized hash).
func isNotFound(err error) bool {
	var rpcErr libdeluge.RPCError
	if errors.As(err, &rpcErr) {
		return strings.Contains(rpcErr.ExceptionType, "InvalidTorrentError")
	}
	return false
}

func convertTorrent(ts *libdeluge.TorrentStatus) TorrentInfo {
	return TorrentInfo{
		Hash:     ts.Hash,
		Name:     ts.Name,
		State:    ts.State,
		Progress: float64(ts.Progress) / 100,
		Size:     ts.TotalSize,
		DlSpeed:  ts.DownloadPayloadRate,
		NumSeeds: ts.NumSeeds,
		SavePath: ts.SavePath,
	}
}

func (c *client) AddMagnet(ctx context.Context, magnet string, opts AddOptions) error {
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return err
	}
	stopped, firstLast, sequential := opts.Stopped, opts.FirstLastPiecePrio, opts.SequentialDownload
	options := &libdeluge.Options{
		AddPaused:                 &stopped,
		PrioritizeFirstLastPieces: &firstLast,
		V2:                        libdeluge.V2Options{SequentialDownload: &sequential},
	}
	if _, err := dc.AddTorrentMagnet(ctx, magnet, options); err != nil {
		c.markDisconnected()
		return fmt.Errorf("deluge add magnet: %w", err)
	}
	return nil
}

func (c *client) Torrents(ctx context.Context, hashes []string) ([]TorrentInfo, error) {
	if len(hashes) == 0 {
		// An empty hash filter means "every torrent on the daemon" to
		// Deluge's RPC, which could include torrents unrelated to
		// seedstrem on a shared instance.
		return nil, nil
	}
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return nil, err
	}
	statuses, err := dc.TorrentsStatus(ctx, libdeluge.StateUnspecified, hashes)
	if err != nil {
		c.markDisconnected()
		return nil, fmt.Errorf("deluge list torrents: %w", err)
	}
	out := make([]TorrentInfo, 0, len(statuses))
	for _, ts := range statuses {
		out = append(out, convertTorrent(ts))
	}
	return out, nil
}

func (c *client) Torrent(ctx context.Context, hash string) (TorrentInfo, error) {
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return TorrentInfo{}, err
	}
	ts, err := dc.TorrentStatus(ctx, hash)
	if err != nil {
		if isNotFound(err) {
			return TorrentInfo{}, ErrTorrentNotFound
		}
		c.markDisconnected()
		return TorrentInfo{}, fmt.Errorf("deluge get torrent %s: %w", hash, err)
	}
	return convertTorrent(ts), nil
}

func (c *client) Files(ctx context.Context, hash string) ([]FileInfo, error) {
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return nil, err
	}
	ts, err := dc.TorrentStatus(ctx, hash)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrTorrentNotFound
		}
		c.markDisconnected()
		return nil, fmt.Errorf("deluge files %s: %w", hash, err)
	}
	infos := make([]FileInfo, 0, len(ts.Files))
	for _, f := range ts.Files {
		info := FileInfo{Index: int(f.Index), Name: f.Path, Size: f.Size}
		if int(f.Index) < len(ts.FileProgress) {
			info.Progress = float64(ts.FileProgress[f.Index])
		}
		if int(f.Index) < len(ts.FilePriorities) {
			info.Priority = int(ts.FilePriorities[f.Index])
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (c *client) Properties(ctx context.Context, hash string) (Properties, error) {
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return Properties{}, err
	}
	ts, err := dc.TorrentStatus(ctx, hash)
	if err != nil {
		if isNotFound(err) {
			return Properties{}, ErrTorrentNotFound
		}
		c.markDisconnected()
		return Properties{}, fmt.Errorf("deluge properties %s: %w", hash, err)
	}
	return Properties{PieceSize: ts.PieceLength, PiecesNum: int(ts.NumPieces), SavePath: ts.SavePath}, nil
}

func (c *client) PieceStates(ctx context.Context, hash string) ([]PieceState, error) {
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := dc.PieceStates(ctx, hash)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrTorrentNotFound
		}
		c.markDisconnected()
		return nil, fmt.Errorf("deluge piece states %s: %w", hash, err)
	}
	out := make([]PieceState, len(raw))
	for i, s := range raw {
		out[i] = PieceState(s)
	}
	return out, nil
}

// SetFilePriority sets priority for the given file indices, leaving
// every other file's priority untouched. Deluge's RPC only supports
// setting the entire per-file priority array at once, so this reads the
// current array, mutates the target indices, and writes it back.
func (c *client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	if len(indices) == 0 {
		return nil
	}
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return err
	}
	ts, err := dc.TorrentStatus(ctx, hash)
	if err != nil {
		if isNotFound(err) {
			return ErrTorrentNotFound
		}
		c.markDisconnected()
		return fmt.Errorf("deluge get status for priority %s: %w", hash, err)
	}
	priorities := make([]int64, len(ts.Files))
	copy(priorities, ts.FilePriorities)
	for _, idx := range indices {
		if idx < 0 || idx >= len(priorities) {
			continue
		}
		priorities[idx] = int64(priority)
	}
	if err := dc.SetFilePriorities(ctx, hash, priorities); err != nil {
		c.markDisconnected()
		return fmt.Errorf("deluge set file priority %s: %w", hash, err)
	}
	return nil
}

func (c *client) Start(ctx context.Context, hash string) error {
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return err
	}
	if err := dc.ResumeTorrents(ctx, hash); err != nil {
		c.markDisconnected()
		return fmt.Errorf("deluge start %s: %w", hash, err)
	}
	return nil
}

func (c *client) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return err
	}
	ok, err := dc.RemoveTorrent(ctx, hash, deleteFiles)
	if err != nil {
		if isNotFound(err) {
			return ErrTorrentNotFound
		}
		c.markDisconnected()
		return fmt.Errorf("deluge delete %s: %w", hash, err)
	}
	if !ok {
		return ErrTorrentNotFound
	}
	return nil
}

func (c *client) Version(ctx context.Context) (string, error) {
	dc, err := c.ensureConnected(ctx)
	if err != nil {
		return "", err
	}
	v, err := dc.DaemonVersion(ctx)
	if err != nil {
		c.markDisconnected()
		return "", fmt.Errorf("deluge version: %w", err)
	}
	return v, nil
}
