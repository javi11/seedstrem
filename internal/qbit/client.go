// Package qbit wraps the qBittorrent WebUI API behind a narrow interface
// tailored to seedstrem's needs. The adapter delegates to
// github.com/autobrr/go-qbittorrent, which handles cookie login,
// re-authentication, and qBittorrent 4.x/5.x endpoint renames.
package qbit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
)

// Client is the qBittorrent surface used by seedstrem.
type Client interface {
	AddMagnet(ctx context.Context, magnet string, opts AddOptions) error
	AddTorrentFile(ctx context.Context, raw []byte, opts AddOptions) error
	Torrents(ctx context.Context, hashes []string) ([]TorrentInfo, error)
	Torrent(ctx context.Context, hash string) (TorrentInfo, error)
	Files(ctx context.Context, hash string) ([]FileInfo, error)
	Properties(ctx context.Context, hash string) (Properties, error)
	PieceStates(ctx context.Context, hash string) ([]PieceState, error)
	SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error
	Start(ctx context.Context, hash string) error
	Delete(ctx context.Context, hash string, deleteFiles bool) error
	AppPreferences(ctx context.Context) (Prefs, error)
	Version(ctx context.Context) (string, error)
}

// ErrTorrentNotFound is returned when qBittorrent does not know the hash.
var ErrTorrentNotFound = fmt.Errorf("torrent not found in qbittorrent")

type client struct {
	qb       *qbt.Client
	category string
}

// New creates a Client for the qBittorrent WebUI at url. Login happens
// lazily inside the underlying library on first use, so New cannot fail —
// matching seedstrem's hot-swappable client pattern. category is the
// default category applied to torrents seedstrem adds.
func New(url, username, password, category string) Client {
	return &client{
		qb: qbt.NewClient(qbt.Config{
			Host:     url,
			Username: username,
			Password: password,
		}),
		category: category,
	}
}

func (c *client) addOptionsMap(opts AddOptions) map[string]string {
	m := map[string]string{}
	category := opts.Category
	if category == "" {
		category = c.category
	}
	if category != "" {
		m["category"] = category
	}
	if opts.Stopped {
		// qBittorrent 5.x uses "stopped", 4.x uses "paused"; unknown form
		// fields are ignored, so send both.
		m["stopped"] = "true"
		m["paused"] = "true"
	}
	if opts.SequentialDownload {
		m["sequentialDownload"] = "true"
	}
	if opts.FirstLastPiecePrio {
		m["firstLastPiecePrio"] = "true"
	}
	return m
}

func (c *client) AddMagnet(ctx context.Context, magnet string, opts AddOptions) error {
	if _, err := c.qb.AddTorrentFromUrlCtx(ctx, magnet, c.addOptionsMap(opts)); err != nil {
		return fmt.Errorf("qbit add magnet: %w", err)
	}
	return nil
}

// AddTorrentFile adds a torrent from raw .torrent bytes. Unlike a magnet,
// the metadata is already present, so qBittorrent skips the metadata
// (metaDL) fetch entirely — essential for private trackers whose peers
// won't reliably serve metadata over the wire.
func (c *client) AddTorrentFile(ctx context.Context, raw []byte, opts AddOptions) error {
	if _, err := c.qb.AddTorrentFromMemoryCtx(ctx, raw, c.addOptionsMap(opts)); err != nil {
		return fmt.Errorf("qbit add torrent file: %w", err)
	}
	return nil
}

func convertTorrent(t qbt.Torrent) TorrentInfo {
	return TorrentInfo{
		Hash:        t.Hash,
		Name:        t.Name,
		State:       normalizeState(string(t.State)),
		Progress:    t.Progress,
		Size:        t.Size,
		DlSpeed:     t.DlSpeed,
		NumSeeds:    t.NumSeeds,
		SavePath:    t.SavePath,
		ContentPath: t.ContentPath,
		SeedingTime: time.Duration(t.SeedingTime) * time.Second,
	}
}

func (c *client) Torrents(ctx context.Context, hashes []string) ([]TorrentInfo, error) {
	if len(hashes) == 0 {
		// An empty filter means "every torrent" to qBittorrent, which could
		// include torrents unrelated to seedstrem on a shared instance.
		return nil, nil
	}
	list, err := c.qb.GetTorrentsCtx(ctx, qbt.TorrentFilterOptions{Hashes: hashes})
	if err != nil {
		return nil, fmt.Errorf("qbit list torrents: %w", err)
	}
	infos := make([]TorrentInfo, 0, len(list))
	for _, t := range list {
		infos = append(infos, convertTorrent(t))
	}
	return infos, nil
}

func (c *client) Torrent(ctx context.Context, hash string) (TorrentInfo, error) {
	list, err := c.qb.GetTorrentsCtx(ctx, qbt.TorrentFilterOptions{Hashes: []string{hash}})
	if err != nil {
		return TorrentInfo{}, fmt.Errorf("qbit get torrent %s: %w", hash, err)
	}
	for _, t := range list {
		if strings.EqualFold(t.Hash, hash) {
			return convertTorrent(t), nil
		}
	}
	return TorrentInfo{}, ErrTorrentNotFound
}

func (c *client) Files(ctx context.Context, hash string) ([]FileInfo, error) {
	files, err := c.qb.GetFilesInformationCtx(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("qbit files %s: %w", hash, err)
	}
	if files == nil {
		return nil, nil
	}
	infos := make([]FileInfo, 0, len(*files))
	for _, f := range *files {
		infos = append(infos, FileInfo{
			Index:    f.Index,
			Name:     f.Name,
			Size:     f.Size,
			Progress: float64(f.Progress),
			Priority: int(f.Priority),
		})
	}
	return infos, nil
}

func (c *client) Properties(ctx context.Context, hash string) (Properties, error) {
	p, err := c.qb.GetTorrentPropertiesCtx(ctx, hash)
	if err != nil {
		return Properties{}, fmt.Errorf("qbit properties %s: %w", hash, err)
	}
	return Properties{
		PieceSize: int64(p.PieceSize),
		PiecesNum: p.PiecesNum,
		SavePath:  p.SavePath,
	}, nil
}

func (c *client) PieceStates(ctx context.Context, hash string) ([]PieceState, error) {
	states, err := c.qb.GetTorrentPieceStatesCtx(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("qbit piece states %s: %w", hash, err)
	}
	out := make([]PieceState, len(states))
	for i, s := range states {
		out[i] = PieceState(s)
	}
	return out, nil
}

func (c *client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	if len(indices) == 0 {
		return nil
	}
	ids := make([]string, len(indices))
	for i, idx := range indices {
		ids[i] = strconv.Itoa(idx)
	}
	if err := c.qb.SetFilePriorityCtx(ctx, hash, strings.Join(ids, "|"), priority); err != nil {
		return fmt.Errorf("qbit set file priority %s: %w", hash, err)
	}
	return nil
}

func (c *client) Start(ctx context.Context, hash string) error {
	if err := c.qb.StartCtx(ctx, []string{hash}); err != nil {
		return fmt.Errorf("qbit start %s: %w", hash, err)
	}
	return nil
}

func (c *client) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	if err := c.qb.DeleteTorrentsCtx(ctx, []string{hash}, deleteFiles); err != nil {
		return fmt.Errorf("qbit delete %s: %w", hash, err)
	}
	return nil
}

// AppPreferences returns the subset of qBittorrent settings needed to
// locate in-progress files (temp download folder, .!qB extension).
func (c *client) AppPreferences(ctx context.Context) (Prefs, error) {
	p, err := c.qb.GetAppPreferencesCtx(ctx)
	if err != nil {
		return Prefs{}, fmt.Errorf("qbit app preferences: %w", err)
	}
	return Prefs{
		TempPath:           p.TempPath,
		TempPathEnabled:    p.TempPathEnabled,
		IncompleteFilesExt: p.IncompleteFilesExt,
	}, nil
}

func (c *client) Version(ctx context.Context) (string, error) {
	v, err := c.qb.GetAppVersionCtx(ctx)
	if err != nil {
		return "", fmt.Errorf("qbit version: %w", err)
	}
	return v, nil
}
