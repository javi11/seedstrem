// Package playsession tracks how many active stream requests reference
// each torrent hash, so cleanup logic can tell whether a torrent is
// currently being watched before removing it.
package playsession

import "sync"

// Sessions is a concurrency-safe refcount of active streaming sessions
// per torrent hash, coordinated with in-progress removals so a torrent
// can't be deleted out from under a session that starts just as
// cleanup decides to remove it.
type Sessions struct {
	mu       sync.Mutex
	active   map[string]int
	removing map[string]chan struct{}
}

// New creates an empty Sessions tracker.
func New() *Sessions {
	return &Sessions{active: map[string]int{}, removing: map[string]chan struct{}{}}
}

// Begin registers one active session for hash. If a removal is
// currently in progress for hash, Begin blocks until it finishes (the
// torrent will then either be gone, or safely available again) before
// registering. The returned end func must be called (typically
// deferred) when the session finishes; it reports how many sessions
// remain for hash afterward.
func (s *Sessions) Begin(hash string) (end func() int) {
	for {
		s.mu.Lock()
		ch, inProgress := s.removing[hash]
		if !inProgress {
			s.active[hash]++
			s.mu.Unlock()
			break
		}
		s.mu.Unlock()
		<-ch
	}

	done := false
	return func() int {
		s.mu.Lock()
		defer s.mu.Unlock()
		if done {
			return s.active[hash]
		}
		done = true
		s.active[hash]--
		n := s.active[hash]
		if n <= 0 {
			delete(s.active, hash)
			n = 0
		}
		return n
	}
}

// Active reports whether hash currently has at least one active
// session.
func (s *Sessions) Active(hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[hash] > 0
}

// BeginRemoval atomically checks that hash has no active sessions and,
// if so, marks it as being removed so any concurrent Begin(hash) calls
// block until the removal attempt finishes. It reports ok=false (no
// done func) if a session is currently active, or a removal is already
// in progress — the caller must abort the removal in that case. When ok
// is true, the caller must call the returned done func exactly once,
// regardless of whether the removal succeeded, to unblock any Begin
// calls that arrived in the meantime.
func (s *Sessions) BeginRemoval(hash string) (done func(), ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[hash] > 0 {
		return nil, false
	}
	if _, exists := s.removing[hash]; exists {
		return nil, false
	}
	ch := make(chan struct{})
	s.removing[hash] = ch
	return func() {
		s.mu.Lock()
		delete(s.removing, hash)
		s.mu.Unlock()
		close(ch)
	}, true
}
