// Package qbit wraps the qBittorrent WebUI API behind a narrow
// interface tailored to seedstrem's needs. The adapter delegates to
// github.com/autobrr/go-qbittorrent, which handles cookie login,
// re-authentication, and qBittorrent 4.x/5.x endpoint renames.
package qbit

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	qbt "github.com/autobrr/go-qbittorrent"
)

// Client is the qBittorrent surface used by seedstrem.
type Client interface {
	AddMagnet(ctx context.Context, magnet string, opts AddOptions) error
	AddTorrentFile(ctx context.Context, raw []byte, opts AddOptions) error
	Torrents(ctx context.Context, category string) ([]TorrentInfo, error)
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
var ErrTorrentNotFound = fmt.Errorf("torrent not found in qBittorrent")

type client struct {
	qb *qbt.Client
}

// New creates a Client for the qBittorrent WebUI at url.
func New(url, username, password string) Client {
	return &client{qb: qbt.NewClient(qbt.Config{
		Host:     url,
		Username: username,
		Password: password,
	})}
}

func addOptionsMap(opts AddOptions) map[string]string {
	m := map[string]string{}
	if opts.Category != "" {
		m["category"] = opts.Category
	}
	if opts.Stopped {
		// qBittorrent 5.x uses "stopped", 4.x uses "paused"; unknown
		// form fields are ignored, so send both.
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
	if _, err := c.qb.AddTorrentFromUrlCtx(ctx, magnet, addOptionsMap(opts)); err != nil {
		return fmt.Errorf("qbit add magnet: %w", err)
	}
	return nil
}

func (c *client) AddTorrentFile(ctx context.Context, raw []byte, opts AddOptions) error {
	if _, err := c.qb.AddTorrentFromMemoryCtx(ctx, raw, addOptionsMap(opts)); err != nil {
		return fmt.Errorf("qbit add torrent file: %w", err)
	}
	return nil
}

func convertTorrent(t qbt.Torrent) TorrentInfo {
	return TorrentInfo{
		Hash:        t.Hash,
		Name:        t.Name,
		State:       string(t.State),
		Progress:    t.Progress,
		Size:        t.Size,
		TotalSize:   t.TotalSize,
		DlSpeed:     t.DlSpeed,
		NumSeeds:    t.NumSeeds,
		SavePath:    t.SavePath,
		ContentPath: t.ContentPath,
		AmountLeft:  t.AmountLeft,
	}
}

func (c *client) Torrents(ctx context.Context, category string) ([]TorrentInfo, error) {
	list, err := c.qb.GetTorrentsCtx(ctx, qbt.TorrentFilterOptions{Category: category})
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
	infos := make([]FileInfo, 0, len(*files))
	for _, f := range *files {
		info := FileInfo{
			Index:    f.Index,
			Name:     f.Name,
			Size:     f.Size,
			Progress: float64(f.Progress),
			Priority: f.Priority,
		}
		if len(f.PieceRange) == 2 {
			info.PieceRange = [2]int{f.PieceRange[0], f.PieceRange[1]}
		}
		infos = append(infos, info)
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
