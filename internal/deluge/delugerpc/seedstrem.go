// seedstrem: local additions to the vendored go-deluge library. This
// file is NOT part of upstream github.com/autobrr/go-deluge; it lives in
// the same package to reach the unexported rpc plumbing. Licensed GPL-2.0
// like the rest of the package (see LICENSE).
package delugerpc

import (
	"context"
	"fmt"

	"github.com/gdm85/go-rencode"
)

// RPC performs a raw RPC call against the daemon and returns the decoded
// return value list. It exposes the existing unexported rpc() so callers
// can reach methods upstream does not wrap — notably custom plugin
// exports such as seedstream.prioritize_range.
func (c *Client) RPC(ctx context.Context, method string, args rencode.List, kwargs rencode.Dictionary) (rencode.List, error) {
	resp, err := c.rpc(ctx, method, args, kwargs)
	if err != nil {
		return rencode.List{}, err
	}
	if resp.IsError() {
		return rencode.List{}, resp.RPCError
	}
	return resp.returnValue, nil
}

// PieceStates returns Deluge's per-piece states for a torrent via the
// "pieces" status key (deluge/core/torrent.py _get_pieces_info):
//
//	0 = missing (no known peer has it / not requested)
//	1 = available in the swarm but not downloaded
//	2 = currently being downloaded from a peer
//	3 = completed
//
// Deluge reports None instead of a list when the torrent has no metadata
// yet or is seeding (finished); that surfaces here as (nil, nil) and the
// caller decides how to synthesize.
func (c *Client) PieceStates(ctx context.Context, hash string) ([]int, error) {
	var args rencode.List
	args.Add(hash)
	args.Add(rencode.NewList("pieces"))

	rd, err := c.rpcWithDictionaryResult(ctx, "core.get_torrent_status", args, rencode.Dictionary{})
	if err != nil {
		return nil, err
	}
	d, err := rd.Zip()
	if err != nil {
		return nil, err
	}
	raw, ok := d["pieces"]
	if !ok || raw == nil {
		return nil, nil
	}
	list, ok := raw.(rencode.List)
	if !ok {
		return nil, fmt.Errorf("deluge pieces: unexpected type %T", raw)
	}
	values := list.Values()
	out := make([]int, len(values))
	for i, v := range values {
		n, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("deluge pieces[%d]: %w", i, err)
		}
		out[i] = n
	}
	return out, nil
}

// SetFilePriorities sets the whole file_priorities array of a torrent
// (Deluge scale: 0 = skip, 1 = low, 4 = normal, 7 = high) via
// core.set_torrent_options, which upstream's Options struct cannot
// express.
func (c *Client) SetFilePriorities(ctx context.Context, hash string, priorities []int) error {
	prioList := rencode.List{}
	for _, p := range priorities {
		prioList.Add(p)
	}
	var options rencode.Dictionary
	options.Add("file_priorities", prioList)

	// core.set_torrent_options takes a LIST of torrent ids.
	var args rencode.List
	args.Add(rencode.NewList(hash), options)

	resp, err := c.rpc(ctx, "core.set_torrent_options", args, rencode.Dictionary{})
	if err != nil {
		return err
	}
	if resp.IsError() {
		return resp.RPCError
	}
	return nil
}

// toInt widens the integer types go-rencode may decode a Python int into.
func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int8:
		return int(n), nil
	case int16:
		return int(n), nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case int:
		return n, nil
	case bool:
		// rencode encodes small Python ints 0/1 distinctly from bools,
		// but be lenient.
		if n {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("not an integer: %T", v)
	}
}
