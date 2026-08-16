package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Existing deployments have no overrides, so the default must be empty and
// resolution must fall through to the global seed time.
func TestDefaultHasNoIndexerSeedTimes(t *testing.T) {
	cfg := Default()
	if len(cfg.Cleanup.IndexerSeedTimes) != 0 {
		t.Errorf("default indexer_seed_times = %v, want empty", cfg.Cleanup.IndexerSeedTimes)
	}
	if got := cfg.Cleanup.EffectiveSeedTime("TorrentLeech"); got != cfg.Cleanup.SeedTime {
		t.Errorf("EffectiveSeedTime = %v, want the global %v", got, cfg.Cleanup.SeedTime)
	}
}

func TestLoadIndexerSeedTimesFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
server:
  listen: ":8080"
  external_url: "http://localhost:8080"
qbittorrent:
  url: "http://localhost:8080"
cleanup:
  seed_time: 72h
  indexer_seed_times:
    TorrentLeech: 240h
    Nyaa: 12h
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, want := cfg.Cleanup.IndexerSeedTimes["TorrentLeech"], 240*time.Hour; got != want {
		t.Errorf("TorrentLeech = %v, want %v", got, want)
	}
	if got, want := cfg.Cleanup.IndexerSeedTimes["Nyaa"], 12*time.Hour; got != want {
		t.Errorf("Nyaa = %v, want %v", got, want)
	}
	if got, want := cfg.Cleanup.EffectiveSeedTime("nyaa"), 12*time.Hour; got != want {
		t.Errorf("EffectiveSeedTime(nyaa) = %v, want %v", got, want)
	}
}

func TestApplyEnvIndexerSeedTimes(t *testing.T) {
	cfg := Default()
	cfg.Cleanup.IndexerSeedTimes = map[string]time.Duration{"Stale": time.Hour}
	applyEnv(&cfg, func(k string) string {
		if k == "SEEDSTREM_CLEANUP_INDEXER_SEED_TIMES" {
			return "TorrentLeech=240h, Nyaa =12h,KeepForever=0s"
		}
		return ""
	})

	// The env value replaces the whole map rather than merging into it.
	if _, ok := cfg.Cleanup.IndexerSeedTimes["Stale"]; ok {
		t.Error("expected the env override to replace the existing map")
	}
	for name, want := range map[string]time.Duration{
		"TorrentLeech": 240 * time.Hour,
		"Nyaa":         12 * time.Hour,
		"KeepForever":  0,
	} {
		if got := cfg.Cleanup.IndexerSeedTimes[name]; got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

// Malformed entries are skipped rather than failing startup, matching the
// other env parsers. The well-formed entries in the same value survive.
func TestApplyEnvIndexerSeedTimesSkipsMalformed(t *testing.T) {
	cfg := Default()
	applyEnv(&cfg, func(k string) string {
		if k == "SEEDSTREM_CLEANUP_INDEXER_SEED_TIMES" {
			return "Good=24h,NoEquals,=12h,Bad=notaduration,  =5h"
		}
		return ""
	})
	want := map[string]time.Duration{"Good": 24 * time.Hour}
	if len(cfg.Cleanup.IndexerSeedTimes) != len(want) {
		t.Fatalf("parsed %v, want only %v", cfg.Cleanup.IndexerSeedTimes, want)
	}
	if got := cfg.Cleanup.IndexerSeedTimes["Good"]; got != 24*time.Hour {
		t.Errorf("Good = %v, want 24h", got)
	}
}

// An unset env var leaves the YAML-loaded map untouched.
func TestApplyEnvIndexerSeedTimesUnsetKeepsYAML(t *testing.T) {
	cfg := Default()
	cfg.Cleanup.IndexerSeedTimes = map[string]time.Duration{"Nyaa": 12 * time.Hour}
	applyEnv(&cfg, func(string) string { return "" })
	if got := cfg.Cleanup.IndexerSeedTimes["Nyaa"]; got != 12*time.Hour {
		t.Errorf("Nyaa = %v, want the YAML value 12h", got)
	}
}

func TestValidateIndexerSeedTimes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		m        map[string]time.Duration
		wantErr  bool
		contains string
	}{
		{name: "nil map is valid"},
		{name: "positive is valid", m: map[string]time.Duration{"Nyaa": 12 * time.Hour}},
		{name: "zero is valid (disables removal)", m: map[string]time.Duration{"Nyaa": 0}},
		{
			name: "negative is rejected", m: map[string]time.Duration{"Nyaa": -time.Hour},
			wantErr: true, contains: `cleanup.indexer_seed_times["Nyaa"]`,
		},
		{
			name: "blank name is rejected", m: map[string]time.Duration{"  ": time.Hour},
			wantErr: true, contains: "blank indexer name",
		},
		{
			// Lookup is case-insensitive, so these two keys would make the
			// effective seed time depend on map iteration order.
			name:    "case-insensitive duplicates are rejected",
			m:       map[string]time.Duration{"Nyaa": time.Hour, "nyaa": 2 * time.Hour},
			wantErr: true, contains: "duplicate entries",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Cleanup.IndexerSeedTimes = tc.m
			err := cfg.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected a validation error")
				}
				if !strings.Contains(err.Error(), tc.contains) {
					t.Errorf("expected error mentioning %q, got: %s", tc.contains, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestHasSeedTimeTrigger(t *testing.T) {
	for _, tc := range []struct {
		name   string
		global time.Duration
		m      map[string]time.Duration
		want   bool
	}{
		{"global only", 72 * time.Hour, nil, true},
		{"nothing set", 0, nil, false},
		{"override only", 0, map[string]time.Duration{"Nyaa": 12 * time.Hour}, true},
		{"all overrides zero", 0, map[string]time.Duration{"Nyaa": 0}, false},
		{"both set", 72 * time.Hour, map[string]time.Duration{"Nyaa": 12 * time.Hour}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasSeedTimeTrigger(tc.global, tc.m); got != tc.want {
				t.Errorf("HasSeedTimeTrigger(%v, %v) = %v, want %v", tc.global, tc.m, got, tc.want)
			}
		})
	}
}
