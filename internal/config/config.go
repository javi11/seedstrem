// Package config defines the seedstrem configuration schema, YAML
// loading with environment-variable overrides, validation, and atomic
// persistence (used by the admin UI to save settings).
package config

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration document.
type Config struct {
	Server   Server   `yaml:"server"`
	Deluge   Deluge   `yaml:"deluge"`
	Prowlarr Prowlarr `yaml:"prowlarr"`
	Addon    Addon    `yaml:"addon"`
	Filters  Filters  `yaml:"filters"`
	Meta     Meta     `yaml:"meta"`
	Paths    Paths    `yaml:"paths"`
	Storage  Storage  `yaml:"storage"`
	Stream   Stream   `yaml:"stream"`
	Log      Log      `yaml:"log"`
}

type Server struct {
	Listen        string `yaml:"listen"`
	ExternalURL   string `yaml:"external_url"`
	AdminPassword string `yaml:"admin_password"`
}

// Deluge configures the RPC connection to the Deluge daemon (not the
// WebUI — seedstrem speaks Deluge's native daemon RPC directly).
type Deluge struct {
	Host     string `yaml:"host"`
	Port     uint   `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Prowlarr configures the torrent search backend.
type Prowlarr struct {
	URL             string `yaml:"url"`
	APIKey          string `yaml:"api_key"`
	MovieCategories []int  `yaml:"movie_categories"`
	TVCategories    []int  `yaml:"tv_categories"`
	AnimeCategories []int  `yaml:"anime_categories"`
	// IndexerIDs scopes searches to specific Prowlarr indexers. Empty
	// means search every enabled indexer.
	IndexerIDs []int `yaml:"indexer_ids"`
}

// Addon toggles which Stremio content types the addon serves.
type Addon struct {
	EnableMovies bool `yaml:"enable_movies"`
	EnableSeries bool `yaml:"enable_series"`
	EnableAnime  bool `yaml:"enable_anime"`
}

// Filters constrains and ranks Prowlarr search results.
type Filters struct {
	MinSeeders int   `yaml:"min_seeders"`
	MinSizeMB  int64 `yaml:"min_size_mb"`
	MaxSizeMB  int64 `yaml:"max_size_mb"` // 0 = unbounded
	MaxResults int   `yaml:"max_results"`
}

// Meta configures metadata resolution (Cinemeta) and the Deluge metadata
// wait during resolve-on-play.
type Meta struct {
	CinemetaURL     string        `yaml:"cinemeta_url"`
	MetadataTimeout time.Duration `yaml:"metadata_timeout"`
	// TMDbAPIKey enables resolving IMDb ids to TMDb ids, for indexers
	// whose Prowlarr definition supports TmdbId search but not ImdbId.
	// Optional: those indexers fall back to free-text search without it.
	TMDbAPIKey string `yaml:"tmdb_api_key"`
}

// Mapping remaps a path prefix as seen by Deluge to a local path.
type Mapping struct {
	Remote string `yaml:"remote"`
	Local  string `yaml:"local"`
}

type Paths struct {
	Mappings []Mapping `yaml:"mappings"`
}

type Storage struct {
	Database            string `yaml:"database"`
	DeleteFilesOnRemove bool   `yaml:"delete_files_on_remove"`
}

type Stream struct {
	WaitTimeout time.Duration `yaml:"wait_timeout"`
	ReadChunk   int64         `yaml:"read_chunk"`
}

type Log struct {
	Level string `yaml:"level"`
}

// Default returns the configuration used when no file or overrides exist.
func Default() Config {
	return Config{
		Server: Server{
			Listen:      ":8080",
			ExternalURL: "http://localhost:8080",
		},
		Deluge: Deluge{
			Host: "deluge",
			Port: 58846,
		},
		Prowlarr: Prowlarr{
			MovieCategories: []int{2000},
			TVCategories:    []int{5000},
			AnimeCategories: []int{5070},
		},
		Addon: Addon{
			EnableMovies: true,
			EnableSeries: true,
			EnableAnime:  false,
		},
		Filters: Filters{
			MinSeeders: 1,
			MaxResults: 20,
		},
		Meta: Meta{
			CinemetaURL:     "https://v3-cinemeta.strem.io",
			MetadataTimeout: 60 * time.Second,
		},
		Paths: Paths{
			Mappings: []Mapping{{Remote: "/downloads", Local: "/data"}},
		},
		Storage: Storage{
			Database:            "seedstrem.db",
			DeleteFilesOnRemove: true,
		},
		Stream: Stream{
			WaitTimeout: 60 * time.Second,
			ReadChunk:   4 << 20,
		},
		Log: Log{Level: "info"},
	}
}

// Load reads the config file at path (if it exists), applies environment
// overrides, and validates the result. A missing file is not an error:
// defaults + env are used.
func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// fine: defaults + env
	default:
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	applyEnv(&cfg, os.Getenv)

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// applyEnv overrides fields from SEEDSTREM_* variables. getenv is
// injectable for tests.
func applyEnv(cfg *Config, getenv func(string) string) {
	set := func(key string, dst *string) {
		if v := getenv("SEEDSTREM_" + key); v != "" {
			*dst = v
		}
	}
	set("SERVER_LISTEN", &cfg.Server.Listen)
	set("SERVER_EXTERNAL_URL", &cfg.Server.ExternalURL)
	set("SERVER_ADMIN_PASSWORD", &cfg.Server.AdminPassword)
	set("DELUGE_HOST", &cfg.Deluge.Host)
	set("DELUGE_USERNAME", &cfg.Deluge.Username)
	set("DELUGE_PASSWORD", &cfg.Deluge.Password)
	if v := getenv("SEEDSTREM_DELUGE_PORT"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 16); err == nil {
			cfg.Deluge.Port = uint(n)
		}
	}
	set("PROWLARR_URL", &cfg.Prowlarr.URL)
	set("PROWLARR_API_KEY", &cfg.Prowlarr.APIKey)
	set("META_CINEMETA_URL", &cfg.Meta.CinemetaURL)
	set("META_TMDB_API_KEY", &cfg.Meta.TMDbAPIKey)
	set("STORAGE_DATABASE", &cfg.Storage.Database)
	set("LOG_LEVEL", &cfg.Log.Level)

	if v := getenv("SEEDSTREM_STREAM_WAIT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Stream.WaitTimeout = d
		}
	}
	if v := getenv("SEEDSTREM_META_METADATA_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Meta.MetadataTimeout = d
		}
	}
	setBool := func(key string, dst *bool) {
		if v := getenv("SEEDSTREM_" + key); v != "" {
			*dst = v == "1" || strings.EqualFold(v, "true")
		}
	}
	setBool("ADDON_ENABLE_MOVIES", &cfg.Addon.EnableMovies)
	setBool("ADDON_ENABLE_SERIES", &cfg.Addon.EnableSeries)
	setBool("ADDON_ENABLE_ANIME", &cfg.Addon.EnableAnime)
	// Comma-separated int lists, e.g. "2000,2010".
	setInts := func(key string, dst *[]int) {
		v := getenv("SEEDSTREM_" + key)
		if v == "" {
			return
		}
		var out []int
		for _, part := range strings.Split(v, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
				out = append(out, n)
			}
		}
		if len(out) > 0 {
			*dst = out
		}
	}
	setInts("PROWLARR_MOVIE_CATEGORIES", &cfg.Prowlarr.MovieCategories)
	setInts("PROWLARR_TV_CATEGORIES", &cfg.Prowlarr.TVCategories)
	setInts("PROWLARR_ANIME_CATEGORIES", &cfg.Prowlarr.AnimeCategories)
	setInts("PROWLARR_INDEXER_IDS", &cfg.Prowlarr.IndexerIDs)
	// SEEDSTREM_PATHS_MAPPINGS: comma-separated "remote:local" pairs,
	// e.g. "/downloads:/data,/media:/mnt/media".
	if v := getenv("SEEDSTREM_PATHS_MAPPINGS"); v != "" {
		var mappings []Mapping
		for _, pair := range strings.Split(v, ",") {
			remote, local, ok := strings.Cut(strings.TrimSpace(pair), ":")
			if ok && remote != "" && local != "" {
				mappings = append(mappings, Mapping{Remote: remote, Local: local})
			}
		}
		if len(mappings) > 0 {
			cfg.Paths.Mappings = mappings
		}
	}
}

// Validate returns all problems found, joined into one error.
func (c Config) Validate() error {
	var errs []error
	if c.Server.Listen == "" {
		errs = append(errs, errors.New("server.listen must not be empty"))
	}
	if c.Server.ExternalURL == "" {
		errs = append(errs, errors.New("server.external_url must not be empty"))
	} else if !strings.HasPrefix(c.Server.ExternalURL, "http://") && !strings.HasPrefix(c.Server.ExternalURL, "https://") {
		errs = append(errs, fmt.Errorf("server.external_url must start with http:// or https://, got %q", c.Server.ExternalURL))
	}
	if c.Deluge.Host == "" {
		errs = append(errs, errors.New("deluge.host must not be empty"))
	}
	if c.Deluge.Port == 0 {
		errs = append(errs, errors.New("deluge.port must not be zero"))
	}
	if c.Storage.Database == "" {
		errs = append(errs, errors.New("storage.database must not be empty"))
	}
	if c.Stream.WaitTimeout <= 0 {
		errs = append(errs, errors.New("stream.wait_timeout must be positive"))
	}
	// Prowlarr may be unconfigured on first boot (set later via the web
	// UI), so only the format is validated here — not presence.
	if c.Prowlarr.URL != "" && !strings.HasPrefix(c.Prowlarr.URL, "http://") && !strings.HasPrefix(c.Prowlarr.URL, "https://") {
		errs = append(errs, fmt.Errorf("prowlarr.url must start with http:// or https://, got %q", c.Prowlarr.URL))
	}
	if c.Meta.CinemetaURL != "" && !strings.HasPrefix(c.Meta.CinemetaURL, "http://") && !strings.HasPrefix(c.Meta.CinemetaURL, "https://") {
		errs = append(errs, fmt.Errorf("meta.cinemeta_url must start with http:// or https://, got %q", c.Meta.CinemetaURL))
	}
	if c.Meta.MetadataTimeout <= 0 {
		errs = append(errs, errors.New("meta.metadata_timeout must be positive"))
	}
	if c.Filters.MinSeeders < 0 {
		errs = append(errs, errors.New("filters.min_seeders must not be negative"))
	}
	if c.Filters.MinSizeMB < 0 || c.Filters.MaxSizeMB < 0 {
		errs = append(errs, errors.New("filters.min_size_mb and max_size_mb must not be negative"))
	}
	if c.Filters.MaxSizeMB != 0 && c.Filters.MaxSizeMB < c.Filters.MinSizeMB {
		errs = append(errs, errors.New("filters.max_size_mb must be >= min_size_mb (or 0 for unbounded)"))
	}
	if c.Filters.MaxResults < 0 {
		errs = append(errs, errors.New("filters.max_results must not be negative"))
	}
	if c.Stream.ReadChunk <= 0 {
		errs = append(errs, errors.New("stream.read_chunk must be positive"))
	}
	for i, m := range c.Paths.Mappings {
		switch {
		case m.Remote == "" || m.Local == "":
			errs = append(errs, fmt.Errorf("paths.mappings[%d]: remote and local must both be set", i))
		case !filepath.IsAbs(m.Local):
			errs = append(errs, fmt.Errorf("paths.mappings[%d]: local %q must be an absolute path", i, m.Local))
		case strings.Contains(m.Local, ".."):
			errs = append(errs, fmt.Errorf("paths.mappings[%d]: local %q must not contain ..", i, m.Local))
		}
	}
	return errors.Join(errs...)
}

// Save writes the config to path atomically (temp file + rename).
func Save(cfg Config, path string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename config into place: %w", err)
	}
	return nil
}

const tokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// GenerateSecret returns a cryptographically random alphanumeric string.
func GenerateSecret(length int) (string, error) {
	var sb strings.Builder
	sb.Grow(length)
	max := big.NewInt(int64(len(tokenAlphabet)))
	for range length {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate secret: %w", err)
		}
		sb.WriteByte(tokenAlphabet[n.Int64()])
	}
	return sb.String(), nil
}

// EnsureSecrets fills in admin_password if empty.
// It reports whether the config was modified.
func EnsureSecrets(cfg *Config) (bool, error) {
	changed := false
	if cfg.Server.AdminPassword == "" {
		pw, err := GenerateSecret(16)
		if err != nil {
			return false, err
		}
		cfg.Server.AdminPassword = pw
		changed = true
	}
	return changed, nil
}
