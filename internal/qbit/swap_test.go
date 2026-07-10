package qbit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/qbit/fake"
)

// TestSwappableDelegatesAndSwaps exercises every delegating method and
// verifies Swap redirects to a new backend.
func TestSwappableDelegatesAndSwaps(t *testing.T) {
	f1 := fake.New()
	t.Cleanup(f1.Close)
	f2 := fake.New()
	t.Cleanup(f2.Close)

	hash := strings.Repeat("c", 40)
	f1.Put(&fake.Torrent{
		Hash: hash, Name: "on-f1", State: "downloading", Category: "seedstrem",
		PieceSize: 1024, PieceStates: []int{2},
		Files: []fake.File{{Name: "a.mkv", Size: 1024, Priority: 1}},
	})
	f2.Put(&fake.Torrent{Hash: hash, Name: "on-f2", State: "downloading", Category: "seedstrem"})

	s := qbit.NewSwappable(qbit.New(f1.URL(), "u", "p"))
	ctx := context.Background()

	tor, err := s.Torrent(ctx, hash)
	if err != nil || tor.Name != "on-f1" {
		t.Fatalf("torrent via f1: %+v %v", tor, err)
	}
	if _, err := s.Torrents(ctx, "seedstrem"); err != nil {
		t.Errorf("Torrents: %v", err)
	}
	if _, err := s.Files(ctx, hash); err != nil {
		t.Errorf("Files: %v", err)
	}
	if _, err := s.Properties(ctx, hash); err != nil {
		t.Errorf("Properties: %v", err)
	}
	if _, err := s.PieceStates(ctx, hash); err != nil {
		t.Errorf("PieceStates: %v", err)
	}
	if err := s.SetFilePriority(ctx, hash, []int{0}, 0); err != nil {
		t.Errorf("SetFilePriority: %v", err)
	}
	if err := s.Start(ctx, hash); err != nil {
		t.Errorf("Start: %v", err)
	}
	if _, err := s.AppPreferences(ctx); err != nil {
		t.Errorf("AppPreferences: %v", err)
	}
	if _, err := s.Version(ctx); err != nil {
		t.Errorf("Version: %v", err)
	}
	magnet := "magnet:?xt=urn:btih:" + strings.Repeat("d", 40)
	if err := s.AddMagnet(ctx, magnet, qbit.AddOptions{Category: "seedstrem"}); err != nil {
		t.Errorf("AddMagnet: %v", err)
	}
	if err := s.AddTorrentFile(ctx, []byte("d4:infod4:name1:aee"), qbit.AddOptions{}); err != nil {
		t.Errorf("AddTorrentFile: %v", err)
	}
	if err := s.Delete(ctx, strings.Repeat("d", 40), false); err != nil {
		t.Errorf("Delete: %v", err)
	}

	// After swap, reads hit f2.
	s.Swap(qbit.New(f2.URL(), "u", "p"))
	tor, err = s.Torrent(ctx, hash)
	if err != nil || tor.Name != "on-f2" {
		t.Fatalf("torrent via f2 after swap: %+v %v", tor, err)
	}
}
