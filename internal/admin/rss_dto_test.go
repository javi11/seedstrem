package admin

import (
	"testing"
	"time"

	"github.com/javib/seedstrem/internal/config"
)

func TestConfigDTORoundTripsRSS(t *testing.T) {
	cfg := config.Default()
	cfg.RSS.Enabled = true
	cfg.RSS.Interval = 20 * time.Minute
	cfg.RSS.MaxGrabsPerCycle = 8
	cfg.RSS.FreeleechOnly = true
	cfg.RSS.Filters = config.RSSFilters{
		MinSizeMB:       500,
		MaxSizeMB:       20000,
		Categories:      []int{2000, 5000},
		IncludeKeywords: []string{"1080p", "2160p"},
		ExcludeKeywords: []string{"CAM"},
	}

	dto := toDTO(cfg)
	if !dto.RSS.Enabled || dto.RSS.IntervalMinutes != 20 || dto.RSS.MaxGrabsPerCycle != 8 || !dto.RSS.FreeleechOnly {
		t.Fatalf("toDTO rss = %+v", dto.RSS)
	}
	if dto.RSS.Filters.MinSizeMB != 500 || dto.RSS.Filters.MaxSizeMB != 20000 {
		t.Fatalf("toDTO rss.filters sizes = %+v", dto.RSS.Filters)
	}
	if len(dto.RSS.Filters.Categories) != 2 || len(dto.RSS.Filters.IncludeKeywords) != 2 || len(dto.RSS.Filters.ExcludeKeywords) != 1 {
		t.Fatalf("toDTO rss.filters lists = %+v", dto.RSS.Filters)
	}

	got := dto.apply(config.Default())
	if !got.RSS.Enabled || got.RSS.Interval != 20*time.Minute || got.RSS.MaxGrabsPerCycle != 8 || !got.RSS.FreeleechOnly {
		t.Fatalf("apply rss = %+v", got.RSS)
	}
	gf := got.RSS.Filters
	if gf.MinSizeMB != 500 || gf.MaxSizeMB != 20000 {
		t.Fatalf("apply rss.filters sizes = %+v", gf)
	}
	if len(gf.Categories) != 2 || gf.Categories[1] != 5000 {
		t.Fatalf("apply rss.filters.categories = %v", gf.Categories)
	}
	if len(gf.IncludeKeywords) != 2 || gf.IncludeKeywords[0] != "1080p" {
		t.Fatalf("apply rss.filters.include_keywords = %v", gf.IncludeKeywords)
	}
	if len(gf.ExcludeKeywords) != 1 || gf.ExcludeKeywords[0] != "CAM" {
		t.Fatalf("apply rss.filters.exclude_keywords = %v", gf.ExcludeKeywords)
	}
}

func TestApplyRSSZeroIntervalKeepsStored(t *testing.T) {
	// A client clearing the interval (0) must not produce an invalid <=0
	// interval — the stored value is kept.
	current := config.Default()
	current.RSS.Interval = 45 * time.Minute

	var dto configDTO
	dto.RSS.IntervalMinutes = 0
	dto.RSS.MaxGrabsPerCycle = 0 // meaningful: disables grabbing, must apply

	got := dto.apply(current)
	if got.RSS.Interval != 45*time.Minute {
		t.Errorf("interval = %v; want kept 45m", got.RSS.Interval)
	}
	if got.RSS.MaxGrabsPerCycle != 0 {
		t.Errorf("max_grabs_per_cycle = %d; want 0 (applied)", got.RSS.MaxGrabsPerCycle)
	}
}
