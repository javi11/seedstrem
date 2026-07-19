package deluge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdm85/go-rencode"

	"github.com/javib/seedstrem/internal/deluge/delugerpc"
	"github.com/javib/seedstrem/internal/downloader"
)

// pluginAPI extends fakeAPI with scriptable RPC responses.
type pluginAPI struct {
	*fakeAPI
	apiVersion    int
	prioritizeErr error
}

func (p *pluginAPI) RPC(_ context.Context, method string, _ rencode.List, _ rencode.Dictionary) (rencode.List, error) {
	p.record("rpc %s", method)
	switch method {
	case "seedstream.api_version":
		return rencode.NewList(int64(p.apiVersion)), nil
	case "seedstream.prioritize_range":
		if p.prioritizeErr != nil {
			return rencode.List{}, p.prioritizeErr
		}
		return rencode.NewList(true), nil
	}
	return rencode.List{}, delugerpc.RPCError{ExceptionType: "AttributeError", ExceptionMessage: "unknown method"}
}

func newPluginClient(p *pluginAPI, at *time.Time) *client {
	return &client{
		rpc:       p,
		label:     "seedstrem",
		flagCache: map[string]flags{},
		now:       func() time.Time { return *at },
	}
}

func countRPC(f *fakeAPI, method string) int {
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, method) {
			n++
		}
	}
	return n
}

func TestPrioritizePiecesWithoutPlugin(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	p := &pluginAPI{fakeAPI: newFakeAPI()} // no plugins enabled
	c := newPluginClient(p, &now)

	err := c.PrioritizePieces(context.Background(), "abc123", 10, 20)
	if !errors.Is(err, downloader.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
	// The negative probe is cached: an immediate second call must not
	// re-list plugins.
	_ = c.PrioritizePieces(context.Background(), "abc123", 10, 20)
	if n := countRPC(p.fakeAPI, "api_version"); n != 0 {
		t.Errorf("api_version probed %d times without the plugin in the list", n)
	}

	// Past the TTL, enabling the plugin is picked up.
	p.plugins = []string{"Label", "Seedstream"}
	p.apiVersion = 1
	now = now.Add(pluginProbeTTL + time.Second)
	if err := c.PrioritizePieces(context.Background(), "abc123", 10, 20); err != nil {
		t.Fatalf("err = %v after enabling plugin", err)
	}
	if n := countRPC(p.fakeAPI, "prioritize_range"); n != 1 {
		t.Errorf("prioritize_range called %d times, want 1", n)
	}
}

func TestPrioritizePiecesNegativeProbeExpiresSooner(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	p := &pluginAPI{fakeAPI: newFakeAPI()} // no plugins enabled
	c := newPluginClient(p, &now)

	if err := c.PrioritizePieces(context.Background(), "abc123", 0, 5); !errors.Is(err, downloader.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}

	// The plugin is enabled while a piece wait is running: the negative
	// probe must expire on its shorter TTL — well before the positive
	// pluginProbeTTL — so the wait's periodic re-hint gets through.
	p.plugins = []string{"Seedstream"}
	p.apiVersion = 1
	now = now.Add(pluginProbeNegTTL + time.Second)
	if err := c.PrioritizePieces(context.Background(), "abc123", 0, 5); err != nil {
		t.Fatalf("err = %v after enabling plugin inside pluginProbeTTL", err)
	}
	if n := countRPC(p.fakeAPI, "prioritize_range"); n != 1 {
		t.Errorf("prioritize_range called %d times, want 1", n)
	}
}

func TestPrioritizePiecesCachesPositiveProbe(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	p := &pluginAPI{fakeAPI: newFakeAPI(), apiVersion: 1}
	p.plugins = []string{"Seedstream"}
	c := newPluginClient(p, &now)

	for range 3 {
		if err := c.PrioritizePieces(context.Background(), "abc123", 0, 5); err != nil {
			t.Fatal(err)
		}
	}
	if n := countRPC(p.fakeAPI, "api_version"); n != 1 {
		t.Errorf("api_version probed %d times, want 1 (cached)", n)
	}
	if n := countRPC(p.fakeAPI, "prioritize_range"); n != 3 {
		t.Errorf("prioritize_range called %d times, want 3", n)
	}
}

func TestPrioritizePiecesRPCErrorInvalidatesProbe(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	p := &pluginAPI{fakeAPI: newFakeAPI(), apiVersion: 1}
	p.plugins = []string{"Seedstream"}
	c := newPluginClient(p, &now)

	if err := c.PrioritizePieces(context.Background(), "abc123", 0, 5); err != nil {
		t.Fatal(err)
	}
	// The plugin gets disabled mid-flight: the daemon-side error must
	// surface as ErrNotSupported and force a fresh probe next call.
	p.prioritizeErr = delugerpc.RPCError{ExceptionType: "AttributeError", ExceptionMessage: "unknown method"}
	p.plugins = nil
	err := c.PrioritizePieces(context.Background(), "abc123", 0, 5)
	if !errors.Is(err, downloader.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
	probes := countRPC(p.fakeAPI, "api_version")
	err = c.PrioritizePieces(context.Background(), "abc123", 0, 5)
	if !errors.Is(err, downloader.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported after re-probe", err)
	}
	if got := len(p.calls); got == 0 {
		t.Fatal("no calls recorded")
	}
	// Re-probe happened: GetEnabledPlugins consulted again (api_version
	// count unchanged since the plugin vanished from the list).
	if n := countRPC(p.fakeAPI, "api_version"); n != probes {
		t.Errorf("api_version count changed unexpectedly: %d -> %d", probes, n)
	}
}

func TestPrioritizePiecesOldAPIVersionUnsupported(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	p := &pluginAPI{fakeAPI: newFakeAPI(), apiVersion: 0}
	p.plugins = []string{"Seedstream"}
	c := newPluginClient(p, &now)

	if err := c.PrioritizePieces(context.Background(), "abc123", 0, 5); !errors.Is(err, downloader.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported for api_version 0", err)
	}
}
