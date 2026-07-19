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

// Kept short: cache TTL + poll interval bound how stale a "piece not
// here yet" answer can be, and that staleness is added on top of every
// piece wait during playback startup (head, MKV tail index, seeks).
// pieceStates is a cheap WebUI call, so poll aggressively.
const (
	pieceCacheTTL = 300 * time.Millisecond
	pollInterval  = 300 * time.Millisecond
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

// Summary is a diagnostic snapshot of a torrent's piece bitfield.
type Summary struct {
	TotalPieces  int // len of the piece-states array qBittorrent reported
	Have         int
	Downloading  int
	FirstMissing int // index of the first piece not yet downloaded (the sequential frontier), -1 if all downloaded
	// HeadState is the worst state among the pieces [headFirst, headLast]
	// (missing < downloading < have). LastState is the final piece's state.
	// Pieces beyond the known bitfield report as missing.
	HeadState qbit.PieceState
	LastState qbit.PieceState
}

// Summary reads the (cached) piece states and condenses them for
// instrumentation: how far the download frontier is, and whether the
// head/tail pieces a player needs are missing, in flight, or on disk.
func (a *Availability) Summary(ctx context.Context, hash string, headFirst, headLast int) (Summary, error) {
	states, err := a.states(ctx, hash)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{TotalPieces: len(states), FirstMissing: -1, HeadState: qbit.PieceHave}
	for i, st := range states {
		switch st {
		case qbit.PieceHave:
			sum.Have++
		case qbit.PieceDownloading:
			sum.Downloading++
		}
		if st != qbit.PieceHave && sum.FirstMissing == -1 {
			sum.FirstMissing = i
		}
	}
	for i := headFirst; i <= headLast; i++ {
		st := qbit.PieceMissing
		if i >= 0 && i < len(states) {
			st = states[i]
		}
		if st < sum.HeadState {
			sum.HeadState = st
		}
	}
	if len(states) > 0 {
		sum.LastState = states[len(states)-1]
	}
	return sum, nil
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
