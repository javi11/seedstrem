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

	dto := toDTO(cfg)
	if !dto.RSS.Enabled || dto.RSS.IntervalMinutes != 20 || dto.RSS.MaxGrabsPerCycle != 8 || !dto.RSS.FreeleechOnly {
		t.Fatalf("toDTO rss = %+v", dto.RSS)
	}

	got := dto.apply(config.Default())
	if !got.RSS.Enabled || got.RSS.Interval != 20*time.Minute || got.RSS.MaxGrabsPerCycle != 8 || !got.RSS.FreeleechOnly {
		t.Fatalf("apply rss = %+v", got.RSS)
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
