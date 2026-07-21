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
