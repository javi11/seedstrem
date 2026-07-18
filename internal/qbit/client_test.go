package qbit

import (
	"testing"
	"time"

	qbt "github.com/autobrr/go-qbittorrent"
)

func TestConvertTorrent(t *testing.T) {
	info := convertTorrent(qbt.Torrent{
		Hash:        "abc123",
		Name:        "Movie",
		State:       qbt.TorrentState("downloading"),
		Progress:    0.42,
		Size:        1000,
		DlSpeed:     500,
		NumSeeds:    7,
		SavePath:    "/downloads",
		SeedingTime: 3600,
	})
	if info.Hash != "abc123" || info.Name != "Movie" {
		t.Errorf("basic fields lost: %+v", info)
	}
	if info.State != StateDownloading {
		t.Errorf("state = %q, want %q", info.State, StateDownloading)
	}
	if info.Progress != 0.42 || info.Size != 1000 || info.DlSpeed != 500 || info.NumSeeds != 7 || info.SavePath != "/downloads" {
		t.Errorf("fields lost: %+v", info)
	}
	if info.SeedingTime != time.Hour {
		t.Errorf("seeding time = %v, want 1h (converted from seconds)", info.SeedingTime)
	}
}

func TestNormalizeState(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"error", StateError},
		{"missingFiles", StateError},
		{"downloading", StateDownloading},
		{"forcedDL", StateDownloading},
		{"metaDL", StateDownloading},
		{"stalledDL", StateDownloading},
		{"uploading", StateSeeding},
		{"stalledUP", StateSeeding},
		{"forcedUP", StateSeeding},
		{"pausedDL", StatePaused},
		{"pausedUP", StatePaused},
		{"stoppedDL", StatePaused},
		{"stoppedUP", StatePaused},
		{"queuedDL", StateQueued},
		{"queuedUP", StateQueued},
		{"checkingDL", StateChecking},
		{"checkingUP", StateChecking},
		{"checkingResumeData", StateChecking},
		{"allocating", StateAllocating},
		{"moving", StateMoving},
		{"unknown", StateDownloading},
		{"something-new", StateDownloading},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := normalizeState(tt.raw); got != tt.want {
				t.Errorf("normalizeState(%q) = %q; want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestAddOptionsMap(t *testing.T) {
	c := &client{category: "default-cat"}

	// Client's configured category is used when opts.Category is empty.
	m := c.addOptionsMap(AddOptions{Stopped: true, SequentialDownload: true, FirstLastPiecePrio: true})
	if m["category"] != "default-cat" {
		t.Errorf("category = %q, want default-cat", m["category"])
	}
	if m["stopped"] != "true" || m["paused"] != "true" {
		t.Errorf("stopped/paused not both set: %v", m)
	}
	if m["sequentialDownload"] != "true" || m["firstLastPiecePrio"] != "true" {
		t.Errorf("sequential/firstLast not set: %v", m)
	}

	// Explicit opts.Category overrides the client default.
	m = c.addOptionsMap(AddOptions{Category: "explicit"})
	if m["category"] != "explicit" {
		t.Errorf("category = %q, want explicit", m["category"])
	}
	if _, ok := m["stopped"]; ok {
		t.Errorf("stopped should be absent when not requested: %v", m)
	}
}
