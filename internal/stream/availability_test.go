package stream

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/qbit/fake"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"

func newAvail(t *testing.T) (*Availability, *fake.Server) {
	t.Helper()
	f := fake.New()
	return NewAvailability(f), f
}

func TestHaveRange(t *testing.T) {
	a, f := newAvail(t)
	f.Put(&fake.Torrent{Hash: testHash, PieceStates: []int{2, 2, 1, 0}})

	ctx := context.Background()
	tests := []struct {
		first, last int
		want        bool
		wantErr     bool
	}{
		{0, 1, true, false},
		{0, 2, false, false},
		{3, 3, false, false},
		{0, 10, false, false}, // past known states → not ready yet (buffer), not an error
		{-1, 0, false, true},  // negative index is a real error
	}
	for _, tt := range tests {
		// Fresh availability per case to defeat the 1s cache when the
		// fake state changes between cases (it doesn't here, but keeps
		// the test honest).
		have, err := a.HaveRange(ctx, testHash, tt.first, tt.last)
		if tt.wantErr {
			if err == nil {
				t.Errorf("HaveRange(%d,%d): expected error", tt.first, tt.last)
			}
			continue
		}
		if err != nil {
			t.Fatalf("HaveRange(%d,%d): %v", tt.first, tt.last, err)
		}
		if have != tt.want {
			t.Errorf("HaveRange(%d,%d) = %v; want %v", tt.first, tt.last, have, tt.want)
		}
	}
}

func TestWaitForRangeSucceedsWhenPiecesArrive(t *testing.T) {
	a, f := newAvail(t)
	f.Put(&fake.Torrent{Hash: testHash, PieceStates: []int{0, 0}})

	// Deterministic clock: each sleep advances fake time and flips a
	// piece, simulating download progress.
	now := time.Unix(1000, 0)
	a.now = func() time.Time { return now }
	var sleeps atomic.Int32
	a.sleep = func(_ context.Context, d time.Duration) error {
		now = now.Add(d)
		n := sleeps.Add(1)
		if n == 2 {
			f.Update(testHash, func(tor *fake.Torrent) { tor.PieceStates = []int{2, 2} })
		}
		return nil
	}

	err := a.WaitForRange(context.Background(), testHash, 0, 1, 10*time.Second)
	if err != nil {
		t.Fatalf("WaitForRange: %v", err)
	}
	if sleeps.Load() < 2 {
		t.Errorf("expected at least 2 polls, got %d", sleeps.Load())
	}
}

func TestWaitForRangeTimesOut(t *testing.T) {
	a, f := newAvail(t)
	f.Put(&fake.Torrent{Hash: testHash, PieceStates: []int{0}})

	now := time.Unix(1000, 0)
	a.now = func() time.Time { return now }
	a.sleep = func(_ context.Context, d time.Duration) error {
		now = now.Add(d)
		return nil
	}

	err := a.WaitForRange(context.Background(), testHash, 0, 0, 3*time.Second)
	if !errors.Is(err, ErrWaitTimeout) {
		t.Errorf("want ErrWaitTimeout, got %v", err)
	}
}

func TestWaitForRangeContextCancelled(t *testing.T) {
	a, f := newAvail(t)
	f.Put(&fake.Torrent{Hash: testHash, PieceStates: []int{0}})

	ctx, cancel := context.WithCancel(context.Background())
	a.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	err := a.WaitForRange(ctx, testHash, 0, 0, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
}

func TestStatesCacheReducesFetches(t *testing.T) {
	a, f := newAvail(t)
	f.Put(&fake.Torrent{Hash: testHash, PieceStates: []int{2}})

	ctx := context.Background()
	for range 5 {
		if _, err := a.HaveRange(ctx, testHash, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	// The fake doesn't count reads, but a wrong cache would show up as
	// an error after Forget + server-side removal.
	a.Forget(testHash)
	f.Remove(testHash)
	if _, err := a.HaveRange(ctx, testHash, 0, 0); err == nil {
		t.Error("expected error after torrent removed and cache forgotten")
	} else if !strings.Contains(err.Error(), "piece states") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSummary(t *testing.T) {
	a, f := newAvail(t)
	// pieces: 0=have 1=downloading 2=missing 3=have 4=missing(last)
	f.Put(&fake.Torrent{Hash: testHash, PieceStates: []int{2, 1, 0, 2, 0}})

	sum, err := a.Summary(context.Background(), testHash, 0, 1)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.TotalPieces != 5 || sum.Have != 2 || sum.Downloading != 1 {
		t.Errorf("counts = %+v, want total=5 have=2 downloading=1", sum)
	}
	if sum.FirstMissing != 1 {
		t.Errorf("frontier = %d, want 1 (first non-have piece)", sum.FirstMissing)
	}
	if got := pieceStateName(sum.HeadState); got != "downloading" {
		t.Errorf("head state = %q, want downloading (worst of pieces 0-1)", got)
	}
	if got := pieceStateName(sum.LastState); got != "missing" {
		t.Errorf("last state = %q, want missing", got)
	}
}

func TestSummaryHeadBeyondKnownBitfield(t *testing.T) {
	a, f := newAvail(t)
	f.Put(&fake.Torrent{Hash: testHash, PieceStates: []int{2}})

	sum, err := a.Summary(context.Background(), testHash, 0, 3)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got := pieceStateName(sum.HeadState); got != "missing" {
		t.Errorf("head state past bitfield = %q, want missing", got)
	}
	if sum.FirstMissing != -1 {
		t.Errorf("frontier = %d, want -1 (all known pieces downloaded)", sum.FirstMissing)
	}
}
