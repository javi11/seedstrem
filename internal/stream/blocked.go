package stream

import "sync"

// blockedRange is the piece range a serve's reader is currently blocked
// on, published for the heartbeat's stall detector.
//
// The detector cannot use a fixed range: it used to watch the file's
// head pieces, which are on disk from the moment playback starts and
// stay there, so after the first read it could never observe a stall
// again — a starving mid-file reader looked identical to a healthy one.
// What is actually stuck is wherever the reader is waiting right now,
// and only the reader knows that.
//
// The zero value is usable and means "nothing blocked". All methods are
// nil-safe so a serve without a heartbeat can leave it unset.
type blockedRange struct {
	mu          sync.Mutex
	first, last int
	waiting     bool
}

// enter publishes [first, last] as the range being awaited.
func (b *blockedRange) enter(first, last int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.first, b.last, b.waiting = first, last, true
}

// leave clears the awaited range: the read went through.
func (b *blockedRange) leave() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.waiting = false
}

// get returns the awaited range and whether a reader is blocked at all.
func (b *blockedRange) get() (first, last int, waiting bool) {
	if b == nil {
		return 0, 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.first, b.last, b.waiting
}
