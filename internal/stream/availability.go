package stream

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/javib/seedstrem/internal/qbit"
)

// ErrWaitTimeout is returned when pieces did not arrive in time.
var ErrWaitTimeout = errors.New("timed out waiting for pieces")

const (
	pieceCacheTTL = 1 * time.Second
	pollInterval  = 1 * time.Second
)

// Availability answers "are these pieces on disk yet?" with a short-TTL
// cache shared across concurrent readers of the same torrent.
type Availability struct {
	dc qbit.Client

	mu    sync.Mutex
	cache map[string]*pieceEntry

	// injectable for tests
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

type pieceEntry struct {
	states    []qbit.PieceState
	fetchedAt time.Time
	inflight  chan struct{} // non-nil while a fetch is running
}

// NewAvailability creates an Availability backed by dc.
func NewAvailability(dc qbit.Client) *Availability {
	return &Availability{
		dc:    dc,
		cache: map[string]*pieceEntry{},
		now:   time.Now,
		sleep: sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// states returns cached piece states, fetching at most once per TTL per
// torrent even under concurrent readers.
func (a *Availability) states(ctx context.Context, hash string) ([]qbit.PieceState, error) {
	for {
		a.mu.Lock()
		entry, ok := a.cache[hash]
		if ok && entry.inflight == nil && a.now().Sub(entry.fetchedAt) < pieceCacheTTL {
			states := entry.states
			a.mu.Unlock()
			return states, nil
		}
		if ok && entry.inflight != nil {
			ch := entry.inflight
			a.mu.Unlock()
			select {
			case <-ch:
				continue // re-check cache
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		// We fetch.
		ch := make(chan struct{})
		a.cache[hash] = &pieceEntry{inflight: ch}
		a.mu.Unlock()

		states, err := a.dc.PieceStates(ctx, hash)

		a.mu.Lock()
		if err != nil {
			delete(a.cache, hash)
		} else {
			a.cache[hash] = &pieceEntry{states: states, fetchedAt: a.now()}
		}
		a.mu.Unlock()
		close(ch)

		if err != nil {
			return nil, fmt.Errorf("piece states %s: %w", hash, err)
		}
		return states, nil
	}
}

// HaveRange reports whether pieces [first, last] are all downloaded.
//
// A range extending past the currently known piece states is reported as
// "not available yet" (false, nil) rather than an error: right after a
// torrent is added qBittorrent has not computed the full piece bitfield,
// so a stream/seek request arriving in that window should buffer (keep
// polling) instead of failing hard. This is what makes the first play
// wait gracefully rather than erroring until a manual retry.
func (a *Availability) HaveRange(ctx context.Context, hash string, first, last int) (bool, error) {
	states, err := a.states(ctx, hash)
	if err != nil {
		return false, err
	}
	if first < 0 {
		return false, fmt.Errorf("negative piece index %d", first)
	}
	if last >= len(states) {
		return false, nil
	}
	for i := first; i <= last; i++ {
		if states[i] != qbit.PieceHave {
			return false, nil
		}
	}
	return true, nil
}

// WaitForRange blocks until pieces [first, last] are downloaded, the
// timeout elapses (ErrWaitTimeout), or ctx is cancelled.
func (a *Availability) WaitForRange(ctx context.Context, hash string, first, last int, timeout time.Duration) error {
	deadline := a.now().Add(timeout)
	for {
		have, err := a.HaveRange(ctx, hash, first, last)
		if err != nil {
			return err
		}
		if have {
			return nil
		}
		if !a.now().Before(deadline) {
			return ErrWaitTimeout
		}
		if err := a.sleep(ctx, pollInterval); err != nil {
			return err
		}
	}
}

// Forget drops cached state for a torrent (e.g. after deletion).
func (a *Availability) Forget(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.cache, hash)
}
