package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Phase values for Torrent.Phase.
const (
	PhaseAdded    = "added"
	PhaseSelected = "selected"
)

// ErrNotFound is returned when a torrent or link does not exist.
var ErrNotFound = errors.New("not found")

// Torrent is a persisted RD-id ↔ qBittorrent-hash mapping. The Content*
// / Season / Episode fields record the Stremio content this torrent was
// added for (empty/zero when unknown), used to surface already-owned
// torrents on later stream requests.
type Torrent struct {
	ID            string
	Hash          string
	Name          string
	Phase         string
	AddedAt       int64 // unix seconds
	Magnet        string
	Error         string
	ContentSource string // "tt", "kitsu", ... ("" when unknown)
	ContentRef    string // id portion, e.g. "tt0944947" ("" when unknown)
	Season        int    // 0 for movies / anime absolute numbering
	Episode       int    // 0 for movies
}

const torrentCols = `id, hash, name, phase, added_at, magnet, error, content_source, content_ref, season, episode`

func scanTorrent(row interface{ Scan(...any) error }) (Torrent, error) {
	var t Torrent
	err := row.Scan(&t.ID, &t.Hash, &t.Name, &t.Phase, &t.AddedAt, &t.Magnet, &t.Error,
		&t.ContentSource, &t.ContentRef, &t.Season, &t.Episode)
	if errors.Is(err, sql.ErrNoRows) {
		return Torrent{}, ErrNotFound
	}
	if err != nil {
		return Torrent{}, fmt.Errorf("scan torrent: %w", err)
	}
	return t, nil
}

// InsertTorrent stores a new torrent row.
func (s *Store) InsertTorrent(ctx context.Context, t Torrent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO torrents (`+torrentCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Hash, t.Name, t.Phase, t.AddedAt, t.Magnet, t.Error,
		t.ContentSource, t.ContentRef, t.Season, t.Episode)
	if err != nil {
		return fmt.Errorf("insert torrent %s: %w", t.ID, err)
	}
	return nil
}

// TorrentByID fetches a torrent by RD id.
func (s *Store) TorrentByID(ctx context.Context, id string) (Torrent, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+torrentCols+` FROM torrents WHERE id = ?`, id)
	return scanTorrent(row)
}

// TorrentByHash fetches a torrent by infohash.
func (s *Store) TorrentByHash(ctx context.Context, hash string) (Torrent, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+torrentCols+` FROM torrents WHERE hash = ?`, hash)
	return scanTorrent(row)
}

// TorrentsByContent returns torrents added for exactly this Stremio
// content identity (source, ref, season, episode), newest-first. Movies
// and anime are stored with season/episode 0, so an exact match on all
// four columns serves every content type. A blank ref yields no rows
// (legacy pre-migration rows have empty content columns and must not
// match every request).
func (s *Store) TorrentsByContent(ctx context.Context, source, ref string, season, episode int) ([]Torrent, error) {
	if ref == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+torrentCols+` FROM torrents
		 WHERE content_source = ? AND content_ref = ? AND season = ? AND episode = ?
		 ORDER BY added_at DESC, id`,
		source, ref, season, episode)
	if err != nil {
		return nil, fmt.Errorf("torrents by content %s/%s: %w", source, ref, err)
	}
	defer rows.Close()

	var torrents []Torrent
	for rows.Next() {
		t, err := scanTorrent(rows)
		if err != nil {
			return nil, err
		}
		torrents = append(torrents, t)
	}
	return torrents, rows.Err()
}

// TorrentsByHashes returns the stored torrents whose infohash is in
// hashes, keyed by lowercase infohash. Hashes not present in the store are
// simply absent from the map. Used to surface already-downloaded torrents
// as cached streams when a Prowlarr search turns up a release we already
// own (e.g. one grabbed in the background by the RSS poller), independent
// of any Stremio content identity.
func (s *Store) TorrentsByHashes(ctx context.Context, hashes []string) (map[string]Torrent, error) {
	out := make(map[string]Torrent, len(hashes))
	// Dedupe and drop empties so the IN clause stays minimal and callers can
	// pass raw, unfiltered result hashes safely.
	seen := make(map[string]struct{}, len(hashes))
	placeholders := make([]string, 0, len(hashes))
	args := make([]any, 0, len(hashes))
	for _, h := range hashes {
		lh := strings.ToLower(h)
		if lh == "" {
			continue
		}
		if _, dup := seen[lh]; dup {
			continue
		}
		seen[lh] = struct{}{}
		placeholders = append(placeholders, "?")
		args = append(args, lh)
	}
	if len(args) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+torrentCols+` FROM torrents WHERE hash IN (`+strings.Join(placeholders, ",")+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("torrents by hashes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		t, err := scanTorrent(rows)
		if err != nil {
			return nil, err
		}
		out[strings.ToLower(t.Hash)] = t
	}
	return out, rows.Err()
}

// SetTorrentContent records the Stremio content identity a torrent was
// added for (used to backfill rows added before the identity was known).
func (s *Store) SetTorrentContent(ctx context.Context, id, source, ref string, season, episode int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE torrents SET content_source = ?, content_ref = ?, season = ?, episode = ? WHERE id = ?`,
		source, ref, season, episode, id)
	if err != nil {
		return fmt.Errorf("set torrent content %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListTorrents returns a page of torrents ordered newest-first, plus the
// total row count (for the X-Total-Count header).
func (s *Store) ListTorrents(ctx context.Context, limit, offset int) ([]Torrent, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM torrents`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count torrents: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+torrentCols+` FROM torrents ORDER BY added_at DESC, id LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list torrents: %w", err)
	}
	defer rows.Close()

	torrents := make([]Torrent, 0, limit)
	for rows.Next() {
		t, err := scanTorrent(rows)
		if err != nil {
			return nil, 0, err
		}
		torrents = append(torrents, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return torrents, total, nil
}

// AllTorrents returns every torrent row (used by the syncer).
func (s *Store) AllTorrents(ctx context.Context) ([]Torrent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+torrentCols+` FROM torrents`)
	if err != nil {
		return nil, fmt.Errorf("all torrents: %w", err)
	}
	defer rows.Close()

	var torrents []Torrent
	for rows.Next() {
		t, err := scanTorrent(rows)
		if err != nil {
			return nil, err
		}
		torrents = append(torrents, t)
	}
	return torrents, rows.Err()
}

// SetTorrentPhase transitions a torrent's phase.
func (s *Store) SetTorrentPhase(ctx context.Context, id, phase string) error {
	return s.updateTorrentField(ctx, id, `phase`, phase)
}

// SetTorrentError records a sticky error on a torrent.
func (s *Store) SetTorrentError(ctx context.Context, id, msg string) error {
	return s.updateTorrentField(ctx, id, `error`, msg)
}

// SetTorrentName updates the display name (learned from qBittorrent
// after metadata resolves).
func (s *Store) SetTorrentName(ctx context.Context, id, name string) error {
	return s.updateTorrentField(ctx, id, `name`, name)
}

func (s *Store) updateTorrentField(ctx context.Context, id, col, val string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE torrents SET `+col+` = ? WHERE id = ?`, val, id)
	if err != nil {
		return fmt.Errorf("update torrent %s %s: %w", id, col, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTorrent removes a torrent (links cascade).
func (s *Store) DeleteTorrent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM torrents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete torrent %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
