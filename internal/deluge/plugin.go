package deluge

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gdm85/go-rencode"

	"github.com/javib/seedstrem/internal/deluge/delugerpc"
	"github.com/javib/seedstrem/internal/downloader"
)

const (
	// pluginName is the Seedstream plugin's name in Deluge's plugin list;
	// its RPC methods live under the lowercased prefix "seedstream.".
	pluginName = "Seedstream"
	// pluginProbeTTL is how long a probe result is trusted before the
	// plugin list is re-checked, so enabling/disabling the plugin is
	// picked up without restarting seedstrem.
	pluginProbeTTL = 60 * time.Second
)

// pluginAvailable reports whether the Seedstream plugin is enabled and
// speaks a compatible API version, caching the answer for
// pluginProbeTTL. Called while do() holds the client mutex (probing
// performs RPC calls on the shared connection).
func (c *client) pluginAvailable(ctx context.Context) bool {
	c.pluginMu.Lock()
	if !c.pluginChecked.IsZero() && c.now().Sub(c.pluginChecked) < pluginProbeTTL {
		ok := c.pluginOK
		c.pluginMu.Unlock()
		return ok
	}
	c.pluginMu.Unlock()

	ok := c.probePlugin(ctx)
	c.pluginMu.Lock()
	c.pluginOK, c.pluginChecked = ok, c.now()
	c.pluginMu.Unlock()
	return ok
}

func (c *client) invalidatePluginProbe() {
	c.pluginMu.Lock()
	c.pluginChecked = time.Time{}
	c.pluginMu.Unlock()
}

func (c *client) probePlugin(ctx context.Context) bool {
	plugins, err := c.rpc.GetEnabledPlugins(ctx)
	if err != nil || !slices.Contains(plugins, pluginName) {
		return false
	}
	res, err := c.rpc.RPC(ctx, "seedstream.api_version", rencode.List{}, rencode.Dictionary{})
	if err != nil {
		return false
	}
	values := res.Values()
	if len(values) == 0 {
		return false
	}
	v, err := toInt(values[0])
	return err == nil && v >= 1
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
	default:
		return 0, fmt.Errorf("not an integer: %T", v)
	}
}

// PrioritizePieces asks the Seedstream plugin to deadline-fetch pieces
// [first, last]. Without the plugin it reports ErrNotSupported so the
// stream layer backs off (and re-probes after the TTL).
func (c *client) PrioritizePieces(ctx context.Context, hash string, first, last int) error {
	return c.do(ctx, func(ctx context.Context) error {
		if !c.pluginAvailable(ctx) {
			return downloader.ErrNotSupported
		}
		args := rencode.NewList(strings.ToLower(hash), first, last)
		if _, err := c.rpc.RPC(ctx, "seedstream.prioritize_range", args, rencode.Dictionary{}); err != nil {
			if errors.As(err, new(delugerpc.RPCError)) {
				// The daemon answered but the call failed — most likely
				// the plugin was disabled since the last probe. Re-probe
				// on the next call and tell the caller to back off.
				c.invalidatePluginProbe()
				return fmt.Errorf("deluge prioritize pieces %s: %v: %w", hash, err, downloader.ErrNotSupported)
			}
			return fmt.Errorf("deluge prioritize pieces %s: %w", hash, err)
		}
		return nil
	})
}
