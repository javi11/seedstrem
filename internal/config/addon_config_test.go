package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultAddonConfig(t *testing.T) {
	cfg := Default()
	if !cfg.Addon.EnableMovies || !cfg.Addon.EnableSeries {
		t.Errorf("movies/series should default on: %+v", cfg.Addon)
	}
	if cfg.Addon.EnableAnime {
		t.Error("anime should default off")
	}
	if cfg.Meta.CinemetaURL != "https://v3-cinemeta.strem.io" {
		t.Errorf("cinemeta default = %q", cfg.Meta.CinemetaURL)
	}
	if cfg.Meta.MetadataTimeout != 60*time.Second {
		t.Errorf("metadata timeout default = %v", cfg.Meta.MetadataTimeout)
	}
	if cfg.Filters.MinSeeders != 1 || cfg.Filters.MaxResults != 20 {
		t.Errorf("filter defaults = %+v", cfg.Filters)
	}
	if len(cfg.Prowlarr.MovieCategories) != 1 || cfg.Prowlarr.MovieCategories[0] != 2000 {
		t.Errorf("movie categories default = %v", cfg.Prowlarr.MovieCategories)
	}
}

func TestApplyEnvProwlarr(t *testing.T) {
	cfg := Default()
	env := map[string]string{
		"SEEDSTREM_PROWLARR_URL":              "http://prowlarr:9696",
		"SEEDSTREM_PROWLARR_API_KEY":          "secret",
		"SEEDSTREM_PROWLARR_MOVIE_CATEGORIES": "2000,2010",
		"SEEDSTREM_PROWLARR_INDEXER_IDS":      "3,7",
		"SEEDSTREM_ADDON_ENABLE_ANIME":        "true",
		"SEEDSTREM_META_METADATA_TIMEOUT":     "30s",
		"SEEDSTREM_META_TMDB_API_KEY":         "tmdb-secret",
	}
	applyEnv(&cfg, func(k string) string { return env[k] })

	if cfg.Prowlarr.URL != "http://prowlarr:9696" || cfg.Prowlarr.APIKey != "secret" {
		t.Errorf("prowlarr env override failed: %+v", cfg.Prowlarr)
	}
	if len(cfg.Prowlarr.MovieCategories) != 2 || cfg.Prowlarr.MovieCategories[1] != 2010 {
		t.Errorf("movie categories env override failed: %v", cfg.Prowlarr.MovieCategories)
	}
	if len(cfg.Prowlarr.IndexerIDs) != 2 || cfg.Prowlarr.IndexerIDs[0] != 3 || cfg.Prowlarr.IndexerIDs[1] != 7 {
		t.Errorf("indexer ids env override failed: %v", cfg.Prowlarr.IndexerIDs)
	}
	if !cfg.Addon.EnableAnime {
		t.Error("anime enable env override failed")
	}
	if cfg.Meta.MetadataTimeout != 30*time.Second {
		t.Errorf("metadata timeout env override failed: %v", cfg.Meta.MetadataTimeout)
	}
	if cfg.Meta.TMDbAPIKey != "tmdb-secret" {
		t.Errorf("tmdb api key env override failed: %q", cfg.Meta.TMDbAPIKey)
	}
}

func TestValidateProwlarrFormat(t *testing.T) {
	cfg := Default()
	cfg.Prowlarr.URL = "ftp://nope"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "prowlarr.url") {
		t.Errorf("expected prowlarr.url scheme error, got %v", err)
	}
}

func TestValidateAllowsUnconfiguredProwlarr(t *testing.T) {
	cfg := Default() // no prowlarr url set — first-boot state
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should validate even without prowlarr: %v", err)
	}
}

func TestValidateFilterSizeBand(t *testing.T) {
	cfg := Default()
	cfg.Filters.MinSizeMB = 1000
	cfg.Filters.MaxSizeMB = 500
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_size_mb") {
		t.Errorf("expected size band error, got %v", err)
	}
}

func TestSaveRoundTripAddon(t *testing.T) {
	cfg := Default()
	cfg.Prowlarr.URL = "http://prowlarr:9696"
	cfg.Prowlarr.APIKey = "abc"
	cfg.Prowlarr.IndexerIDs = []int{4, 8}
	cfg.Meta.TMDbAPIKey = "tmdb-secret"
	cfg.Addon.EnableAnime = true
	cfg.Filters.Qualities = []string{"1080p", "720p"}

	// Round-trip via marshal/unmarshal by re-loading a written file.
	dir := t.TempDir()
	path := dir + "/c.yaml"
	if err := Save(cfg, path); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Prowlarr.URL != cfg.Prowlarr.URL || got.Prowlarr.APIKey != "abc" {
		t.Errorf("prowlarr round-trip lost: %+v", got.Prowlarr)
	}
	if len(got.Prowlarr.IndexerIDs) != 2 || got.Prowlarr.IndexerIDs[0] != 4 || got.Prowlarr.IndexerIDs[1] != 8 {
		t.Errorf("indexer ids round-trip lost: %v", got.Prowlarr.IndexerIDs)
	}
	if got.Meta.TMDbAPIKey != "tmdb-secret" {
		t.Errorf("tmdb api key round-trip lost: %q", got.Meta.TMDbAPIKey)
	}
	if !got.Addon.EnableAnime {
		t.Error("anime toggle round-trip lost")
	}
	if len(got.Filters.Qualities) != 2 {
		t.Errorf("qualities round-trip lost: %v", got.Filters.Qualities)
	}
}
