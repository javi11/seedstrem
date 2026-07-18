package playsession

import (
	"sync"
	"testing"
	"time"
)

func TestBeginEndRefcount(t *testing.T) {
	s := New()
	if s.Active("h1") {
		t.Fatal("expected no active sessions initially")
	}

	end1 := s.Begin("h1")
	if !s.Active("h1") {
		t.Fatal("expected h1 active after Begin")
	}

	end2 := s.Begin("h1")
	if remaining := end1(); remaining != 1 {
		t.Errorf("end1() = %d, want 1", remaining)
	}
	if !s.Active("h1") {
		t.Error("expected h1 still active with one session remaining")
	}

	if remaining := end2(); remaining != 0 {
		t.Errorf("end2() = %d, want 0", remaining)
	}
	if s.Active("h1") {
		t.Error("expected h1 inactive after both sessions ended")
	}
}

func TestEndIsIdempotent(t *testing.T) {
	s := New()
	end := s.Begin("h1")
	if got := end(); got != 0 {
		t.Fatalf("first end() = %d, want 0", got)
	}
	if got := end(); got != 0 {
		t.Fatalf("second end() = %d, want 0 (idempotent)", got)
	}
}

func TestIndependentHashes(t *testing.T) {
	s := New()
	endA := s.Begin("a")
	s.Begin("b")
	if !s.Active("a") || !s.Active("b") {
		t.Fatal("expected both hashes active")
	}
	endA()
	if s.Active("a") {
		t.Error("expected a inactive")
	}
	if !s.Active("b") {
		t.Error("expected b still active")
	}
}

func TestBeginRemovalRefusedWhenActive(t *testing.T) {
	s := New()
	end := s.Begin("h1")
	defer end()

	if _, ok := s.BeginRemoval("h1"); ok {
		t.Fatal("expected BeginRemoval to refuse while a session is active")
	}
}

func TestBeginRemovalRefusedWhenAlreadyInProgress(t *testing.T) {
	s := New()
	done, ok := s.BeginRemoval("h1")
	if !ok {
		t.Fatal("expected first BeginRemoval to succeed")
	}
	defer done()

	if _, ok := s.BeginRemoval("h1"); ok {
		t.Fatal("expected second BeginRemoval to be refused while one is in progress")
	}
}

func TestBeginBlocksUntilRemovalFinishes(t *testing.T) {
	s := New()
	done, ok := s.BeginRemoval("h1")
	if !ok {
		t.Fatal("expected BeginRemoval to succeed")
	}

	beganCh := make(chan func() int, 1)
	go func() {
		beganCh <- s.Begin("h1")
	}()

	select {
	case <-beganCh:
		t.Fatal("expected Begin to block while removal is in progress")
	case <-time.After(50 * time.Millisecond):
	}

	done() // removal finishes, unblocking Begin

	select {
	case end := <-beganCh:
		end()
	case <-time.After(time.Second):
		t.Fatal("expected Begin to unblock after removal finished")
	}
}

func TestConcurrentBeginEnd(t *testing.T) {
	s := New()
	const n = 100
	var wg sync.WaitGroup
	ends := make([]func() int, 0, n)
	var mu sync.Mutex

	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			end := s.Begin("h1")
			mu.Lock()
			ends = append(ends, end)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if !s.Active("h1") {
		t.Fatal("expected h1 active")
	}

	wg.Add(len(ends))
	for _, end := range ends {
		go func() {
			defer wg.Done()
			end()
		}()
	}
	wg.Wait()

	if s.Active("h1") {
		t.Error("expected h1 inactive after all sessions ended")
	}
}
