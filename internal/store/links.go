package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Link is an opaque streaming token for one selected file of a torrent.
type Link struct {
	Token     string
	TorrentID string
	FileIndex int // qBittorrent 0-based file index
	Path      string
	Bytes     int64
}

// InsertLinks stores links for a torrent in one transaction.
func (s *Store) InsertLinks(ctx context.Context, links []Link) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin insert links: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO links (token, torrent_id, file_index, path, bytes) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert links: %w", err)
	}
	defer stmt.Close()

	for _, l := range links {
		if _, err := stmt.ExecContext(ctx, l.Token, l.TorrentID, l.FileIndex, l.Path, l.Bytes); err != nil {
			return fmt.Errorf("insert link for torrent %s file %d: %w", l.TorrentID, l.FileIndex, err)
		}
	}
	return tx.Commit()
}

// LinkByToken fetches a link by its token.
func (s *Store) LinkByToken(ctx context.Context, token string) (Link, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT token, torrent_id, file_index, path, bytes FROM links WHERE token = ?`, token)
	var l Link
	err := row.Scan(&l.Token, &l.TorrentID, &l.FileIndex, &l.Path, &l.Bytes)
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	if err != nil {
		return Link{}, fmt.Errorf("scan link: %w", err)
	}
	return l, nil
}

// LinksByTorrent returns a torrent's links ordered by file index.
func (s *Store) LinksByTorrent(ctx context.Context, torrentID string) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT token, torrent_id, file_index, path, bytes FROM links
		 WHERE torrent_id = ? ORDER BY file_index`, torrentID)
	if err != nil {
		return nil, fmt.Errorf("links for torrent %s: %w", torrentID, err)
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.Token, &l.TorrentID, &l.FileIndex, &l.Path, &l.Bytes); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}
