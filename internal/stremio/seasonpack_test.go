package stremio

import "testing"

func TestIsSeasonPack(t *testing.T) {
	tests := []struct {
		title  string
		season int
		want   bool
	}{
		// Full-season packs for the requested season.
		{"The.Show.S01.1080p.WEB-DL", 1, true},
		{"The Show Season 1 COMPLETE 720p", 1, true},
		{"The.Show.Season.01.1080p", 1, true},
		{"The.Show.S02.Complete.1080p", 2, true},
		// Episode-range releases are packs.
		{"The.Show.S01E01-E10.1080p.WEB", 1, true},
		{"The.Show.S01.E01-E13", 1, true},
		// Complete-series packs contain every season.
		{"The.Show.Complete.Series.1080p", 1, true},
		{"The.Show.Complete.Series.1080p", 3, true},
		// Single-episode releases are NOT packs.
		{"The.Show.S01E05.1080p", 1, false},
		{"The.Show.1x05.720p", 1, false},
		{"The.Show.S01E05.1080p", 1, false},
		// Packs for a different season don't match.
		{"The.Show.S02.1080p", 1, false},
		{"The.Show.S02E03.1080p", 1, false},
		{"The.Show.Season.2.Complete", 1, false},
	}
	for _, tc := range tests {
		if got := isSeasonPack(tc.title, tc.season); got != tc.want {
			t.Errorf("isSeasonPack(%q, %d) = %v, want %v", tc.title, tc.season, got, tc.want)
		}
	}
}
