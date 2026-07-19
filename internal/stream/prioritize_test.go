package stream

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/downloader"
)

// prioSpy records PrioritizePieces calls with a scriptable error.
type prioSpy struct {
	downloader.Client
	mu    sync.Mutex
	calls []string
	err   error
}

func (s *prioSpy) PrioritizePieces(_ context.Context, hash string, first, last int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, hashRange(hash, first, last))
	return s.err
}

func hashRange(hash string, first, last int) string {
	return hash + ":" + itoa(first) + "-" + itoa(last)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (s *prioSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func TestPrioritizerDedupesIdenticalRange(t *testing.T) {
	spy := &prioSpy{}
	p := newPrioritizer(spy, nil)
	now := time.Unix(1_000_000, 0)
	p.now = func() time.Time { return now }
	ctx := context.Background()

	p.request(ctx, "abc", 10, 20)
	p.request(ctx, "abc", 10, 20) // identical, within interval → dropped
	if spy.count() != 1 {
		t.Fatalf("calls = %d, want 1 (dedupe)", spy.count())
	}

	// A different range (the reader moved) goes through immediately.
	p.request(ctx, "abc", 30, 40)
	if spy.count() != 2 {
		t.Fatalf("calls = %d, want 2 (retarget)", spy.count())
	}

	// The identical range goes through again after the interval.
	now = now.Add(prioMinInterval + time.Millisecond)
	p.request(ctx, "abc", 30, 40)
	if spy.count() != 3 {
		t.Fatalf("calls = %d, want 3 (interval elapsed)", spy.count())
	}
}

func TestPrioritizerBacksOffWhenUnsupported(t *testing.T) {
	spy := &prioSpy{err: downloader.ErrNotSupported}
	p := newPrioritizer(spy, nil)
	now := time.Unix(1_000_000, 0)
	p.now = func() time.Time { return now }
	ctx := context.Background()

	p.request(ctx, "abc", 10, 20)
	p.request(ctx, "abc", 30, 40) // silenced by the backoff
	p.request(ctx, "def", 0, 5)   // other hashes silenced too
	if spy.count() != 1 {
		t.Fatalf("calls = %d, want 1 (unsupported backoff)", spy.count())
	}

	// After the backoff (e.g. hot-swap to a capable backend) it retries.
	spy.err = nil
	now = now.Add(prioUnsupportedBackoff + time.Second)
	p.request(ctx, "abc", 10, 20)
	if spy.count() != 2 {
		t.Fatalf("calls = %d, want 2 (retry after backoff)", spy.count())
	}
	if !strings.HasPrefix(spy.calls[1], "abc:") {
		t.Errorf("unexpected call %q", spy.calls[1])
	}
}

func TestPrioritizerSwallowsOtherErrors(t *testing.T) {
	spy := &prioSpy{err: errors.New("transport down")}
	p := newPrioritizer(spy, nil)
	ctx := context.Background()

	p.request(ctx, "abc", 10, 20) // must not panic or backoff
	p.request(ctx, "abc", 30, 40)
	if spy.count() != 2 {
		t.Fatalf("calls = %d, want 2 (no backoff on transient errors)", spy.count())
	}
}

func TestReadaheadPieces(t *testing.T) {
	if got := readaheadPieces(2 << 20); got != 4 {
		t.Errorf("2MiB pieces → %d, want 4 (8MiB window)", got)
	}
	if got := readaheadPieces(16 << 20); got != 4 {
		t.Errorf("16MiB pieces → %d, want 4 (floor)", got)
	}
	if got := readaheadPieces(1 << 20); got != 8 {
		t.Errorf("1MiB pieces → %d, want 8", got)
	}
	if got := readaheadPieces(0); got != 4 {
		t.Errorf("unknown piece size → %d, want 4", got)
	}
}
