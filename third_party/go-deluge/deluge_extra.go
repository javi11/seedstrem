// Additions to upstream go-deluge (see FORK_NOTES.md): upstream never
// requests Deluge's "pieces" status key, and has no way to set
// per-file download priority. Both are added here using the same
// rpc/rpcWithDictionaryResult pattern upstream already uses internally.
package deluge

import (
	"context"

	"github.com/gdm85/go-rencode"
)

// PieceState mirrors Deluge's per-piece status values (see
// deluge/core/torrent.py _get_pieces_info): 0=missing (no known peer has
// it, or not requested), 1=available (a peer has it, not yet requested),
// 2=downloading, 3=completed. Missing and Available both mean "we don't
// have this piece yet" for streaming purposes, so both map to
// PieceMissing here.
type PieceState int

const (
	PieceMissing PieceState = iota
	PieceDownloading
	PieceHave
)

// rawPieceState is Deluge's raw 0-3 status value for one piece.
type rawPieceState int

const (
	rawPieceMissing rawPieceState = iota
	rawPieceAvailable
	rawPieceDownloading
	rawPieceCompleted
)

func (r rawPieceState) toPieceState() PieceState {
	switch r {
	case rawPieceCompleted:
		return PieceHave
	case rawPieceDownloading:
		return PieceDownloading
	default: // rawPieceMissing, rawPieceAvailable
		return PieceMissing
	}
}

// PieceStates returns the per-piece completion state for the torrent with
// the given hash, one entry per piece. Deluge returns a nil "pieces"
// value when the torrent has no metadata yet or is fully seeding; the
// latter is reported here as every piece being PieceHave (there is
// nothing left to wait for). Returns an error if the torrent isn't known
// or has no metadata yet — callers should treat that as "not ready".
func (c *Client) PieceStates(ctx context.Context, hash string) ([]PieceState, error) {
	var args rencode.List
	args.Add(hash, rencode.NewList("pieces", "num_pieces", "is_seed"))

	rd, err := c.rpcWithDictionaryResult(ctx, "core.get_torrent_status", args, rencode.Dictionary{})
	if err != nil {
		return nil, err
	}

	numPiecesRaw, ok := rd.Get("num_pieces")
	if !ok {
		return nil, ErrInvalidDictionaryResponse
	}
	numPieces, err := toInt64(numPiecesRaw)
	if err != nil {
		return nil, err
	}
	if numPieces <= 0 {
		return nil, ErrInvalidReturnValue
	}

	isSeedRaw, _ := rd.Get("is_seed")
	isSeed, _ := isSeedRaw.(bool)

	piecesRaw, hasPieces := rd.Get("pieces")
	piecesList, isList := piecesRaw.(rencode.List)
	if !hasPieces || !isList {
		if isSeed {
			// Fully seeding: Deluge reports "pieces" as None. Everything
			// is downloaded, so report every piece as complete.
			out := make([]PieceState, numPieces)
			for i := range out {
				out[i] = PieceHave
			}
			return out, nil
		}
		// No metadata yet, or an unexpected shape.
		return nil, ErrInvalidReturnValue
	}

	values := piecesList.Values()
	out := make([]PieceState, len(values))
	for i, v := range values {
		n, err := toInt64(v)
		if err != nil {
			return nil, err
		}
		out[i] = rawPieceState(n).toPieceState()
	}
	return out, nil
}

// SetFilePriorities sets the download priority for every file in the
// torrent with the given hash, in file-index order. priorities uses
// libtorrent's 0-7 scale (0 = don't download); index i corresponds to
// the file at index i in TorrentStatus.Files/FilePriorities.
func (c *Client) SetFilePriorities(ctx context.Context, hash string, priorities []int64) error {
	values := make([]any, len(priorities))
	for i, p := range priorities {
		values[i] = p
	}

	var dict rencode.Dictionary
	dict.Add("file_priorities", rencode.NewList(values...))

	var args rencode.List
	args.Add(hash, dict)

	resp, err := c.rpc(ctx, "core.set_torrent_options", args, rencode.Dictionary{})
	if err != nil {
		return err
	}
	if resp.IsError() {
		return resp.RPCError
	}
	return nil
}

// toInt64 converts a decoded rencode numeric value (which may surface as
// any of Go's fixed-width int types depending on the encoded magnitude)
// to int64.
func toInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int32:
		return int64(n), nil
	case int16:
		return int64(n), nil
	case int8:
		return int64(n), nil
	case int:
		return int64(n), nil
	case uint64:
		return int64(n), nil
	case uint32:
		return int64(n), nil
	default:
		return 0, ErrInvalidReturnValue
	}
}
