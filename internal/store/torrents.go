package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Phase values for Torrent.Phase.
const (
	PhaseAdded    = "added"
	PhaseSelected = "selected"
)

// ErrNotFound is returned when a torrent or link does not exist.
var ErrNotFound = errors.New("not found")

// Torrent is a persisted RD-id ↔ Deluge-hash mapping.
type Torrent struct {
	ID      string
	Hash    string
	Name    string
	Phase   string
	AddedAt int64 // unix seconds
	Magnet  string
	Error   string
}

const torrentCols = `id, hash, name, phase, added_at, magnet, error`

func scanTorrent(row interface{ Scan(...any) error }) (Torrent, error) {
	var t Torrent
	err := row.Scan(&t.ID, &t.Hash, &t.Name, &t.Phase, &t.AddedAt, &t.Magnet, &t.Error)
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
		`INSERT INTO torrents (`+torrentCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Hash, t.Name, t.Phase, t.AddedAt, t.Magnet, t.Error)
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

// SetTorrentName updates the display name (learned from Deluge
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
