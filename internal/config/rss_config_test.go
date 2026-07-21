package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultRSSConfig(t *testing.T) {
	cfg := Default()
	if cfg.RSS.Enabled {
		t.Error("rss should be disabled by default")
	}
	if cfg.RSS.Interval != 15*time.Minute {
		t.Errorf("rss.interval default = %v; want 15m", cfg.RSS.Interval)
	}
	if cfg.RSS.MaxGrabsPerCycle != 5 {
		t.Errorf("rss.max_grabs_per_cycle default = %d; want 5", cfg.RSS.MaxGrabsPerCycle)
	}
	if cfg.RSS.FreeleechOnly {
		t.Error("rss.freeleech_only should default to false")
	}
}

func TestApplyEnvRSS(t *testing.T) {
	cfg := Default()
	env := map[string]string{
		"SEEDSTREM_RSS_ENABLED":             "true",
		"SEEDSTREM_RSS_INTERVAL":            "30m",
		"SEEDSTREM_RSS_MAX_GRABS_PER_CYCLE": "12",
		"SEEDSTREM_RSS_FREELEECH_ONLY":      "1",
	}
	applyEnv(&cfg, func(k string) string { return env[k] })

	if !cfg.RSS.Enabled {
		t.Error("rss.enabled override failed")
	}
	if cfg.RSS.Interval != 30*time.Minute {
		t.Errorf("rss.interval override = %v; want 30m", cfg.RSS.Interval)
	}
	if cfg.RSS.MaxGrabsPerCycle != 12 {
		t.Errorf("rss.max_grabs_per_cycle override = %d; want 12", cfg.RSS.MaxGrabsPerCycle)
	}
	if !cfg.RSS.FreeleechOnly {
		t.Error("rss.freeleech_only override failed")
	}
}

func TestDefaultRSSFilters(t *testing.T) {
	cfg := Default()
	f := cfg.RSS.Filters
	if f.MinSizeMB != 0 || f.MaxSizeMB != 0 {
		t.Errorf("rss.filters sizes default = (%d, %d); want (0, 0)", f.MinSizeMB, f.MaxSizeMB)
	}
	if len(f.Categories) != 0 {
		t.Errorf("rss.filters.categories default = %v; want empty", f.Categories)
	}
	if len(f.IncludeKeywords) != 0 || len(f.ExcludeKeywords) != 0 {
		t.Errorf("rss.filters keyword lists should default empty, got include=%v exclude=%v", f.IncludeKeywords, f.ExcludeKeywords)
	}
}

func TestApplyEnvRSSFilters(t *testing.T) {
	cfg := Default()
	env := map[string]string{
		"SEEDSTREM_RSS_FILTERS_MIN_SIZE_MB":      "500",
		"SEEDSTREM_RSS_FILTERS_MAX_SIZE_MB":      "20000",
		"SEEDSTREM_RSS_FILTERS_CATEGORIES":       "2000, 5000",
		"SEEDSTREM_RSS_FILTERS_INCLUDE_KEYWORDS": "1080p, 2160p",
		"SEEDSTREM_RSS_FILTERS_EXCLUDE_KEYWORDS": "CAM, HDTS",
	}
	applyEnv(&cfg, func(k string) string { return env[k] })

	f := cfg.RSS.Filters
	if f.MinSizeMB != 500 || f.MaxSizeMB != 20000 {
		t.Errorf("rss.filters size override = (%d, %d); want (500, 20000)", f.MinSizeMB, f.MaxSizeMB)
	}
	if len(f.Categories) != 2 || f.Categories[0] != 2000 || f.Categories[1] != 5000 {
		t.Errorf("rss.filters.categories override = %v; want [2000 5000]", f.Categories)
	}
	if len(f.IncludeKeywords) != 2 || f.IncludeKeywords[0] != "1080p" || f.IncludeKeywords[1] != "2160p" {
		t.Errorf("rss.filters.include_keywords override = %v; want [1080p 2160p]", f.IncludeKeywords)
	}
	if len(f.ExcludeKeywords) != 2 || f.ExcludeKeywords[0] != "CAM" || f.ExcludeKeywords[1] != "HDTS" {
		t.Errorf("rss.filters.exclude_keywords override = %v; want [CAM HDTS]", f.ExcludeKeywords)
	}
}

func TestValidateRSSFilters(t *testing.T) {
	cfg := Default()
	cfg.RSS.Filters.MinSizeMB = 5000
	cfg.RSS.Filters.MaxSizeMB = 1000 // max < min: invalid
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for max_size_mb < min_size_mb")
	}
	if !strings.Contains(err.Error(), "rss.filters.max_size_mb") {
		t.Errorf("expected error mentioning rss.filters.max_size_mb, got: %s", err.Error())
	}

	cfg = Default()
	cfg.RSS.Filters.MinSizeMB = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "rss.filters.min_size_mb") {
		t.Errorf("expected negative-size error mentioning rss.filters.min_size_mb, got: %v", err)
	}
}

func TestValidateRSSFiltersUnboundedMaxOK(t *testing.T) {
	cfg := Default()
	cfg.RSS.Filters.MinSizeMB = 500
	cfg.RSS.Filters.MaxSizeMB = 0 // 0 = unbounded, must be accepted
	if err := cfg.Validate(); err != nil {
		t.Fatalf("min set with unbounded max should validate: %v", err)
	}
}

func TestValidateRSS(t *testing.T) {
	cfg := Default()
	cfg.RSS.Enabled = true
	cfg.RSS.Interval = 0
	cfg.RSS.MaxGrabsPerCycle = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	msg := err.Error()
	for _, want := range []string{"rss.interval", "rss.max_grabs_per_cycle"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error mentioning %q, got: %s", want, msg)
		}
	}
}

func TestValidateRSSDisabledIgnoresInterval(t *testing.T) {
	cfg := Default()
	cfg.RSS.Enabled = false
	cfg.RSS.Interval = 0 // irrelevant while disabled
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled rss with zero interval should validate: %v", err)
	}
}
