package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Listen != ":8080" {
		t.Errorf("got listen %q; want :8080", cfg.Server.Listen)
	}
	if cfg.Stream.WaitTimeout != 60*time.Second {
		t.Errorf("got wait_timeout %v; want 60s", cfg.Stream.WaitTimeout)
	}
	if cfg.Cleanup.SeedTime != 72*time.Hour {
		t.Errorf("got cleanup.seed_time %v; want 72h", cfg.Cleanup.SeedTime)
	}
	if cfg.Cleanup.MinProgressForCancel != 0.05 {
		t.Errorf("got cleanup.min_progress_for_cancel %v; want 0.05", cfg.Cleanup.MinProgressForCancel)
	}
}

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `
server:
  listen: ":9090"
  external_url: "https://media.example.com"
qbittorrent:
  url: "http://qbittorrent-host:8080"
  username: "u"
  password: "p"
  category: "seedstrem"
stream:
  wait_timeout: 30s
paths:
  mappings:
    - remote: /dl
      local: /mnt/dl
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Listen != ":9090" {
		t.Errorf("got listen %q; want :9090", cfg.Server.Listen)
	}
	if cfg.Stream.WaitTimeout != 30*time.Second {
		t.Errorf("got wait_timeout %v; want 30s", cfg.Stream.WaitTimeout)
	}
	if cfg.QBittorrent.URL != "http://qbittorrent-host:8080" || cfg.QBittorrent.Category != "seedstrem" {
		t.Errorf("qbittorrent connection not parsed: %+v", cfg.QBittorrent)
	}
	if len(cfg.Paths.Mappings) != 1 || cfg.Paths.Mappings[0].Local != "/mnt/dl" {
		t.Errorf("mappings not parsed: %+v", cfg.Paths.Mappings)
	}
}

func TestApplyEnv(t *testing.T) {
	cfg := Default()
	env := map[string]string{
		"SEEDSTREM_QBITTORRENT_URL":                 "http://other-host:9091",
		"SEEDSTREM_QBITTORRENT_CATEGORY":            "other-cat",
		"SEEDSTREM_STREAM_WAIT_TIMEOUT":             "90s",
		"SEEDSTREM_PATHS_MAPPINGS":                  "/a:/b,/c:/d",
		"SEEDSTREM_CLEANUP_SEED_TIME":               "48h",
		"SEEDSTREM_CLEANUP_MIN_PROGRESS_FOR_CANCEL": "0.1",
	}
	applyEnv(&cfg, func(k string) string { return env[k] })

	if cfg.QBittorrent.URL != "http://other-host:9091" || cfg.QBittorrent.Category != "other-cat" {
		t.Errorf("qbittorrent override failed: %+v", cfg.QBittorrent)
	}
	if cfg.Stream.WaitTimeout != 90*time.Second {
		t.Errorf("wait timeout override failed: %v", cfg.Stream.WaitTimeout)
	}
	if len(cfg.Paths.Mappings) != 2 || cfg.Paths.Mappings[1].Remote != "/c" {
		t.Errorf("mappings override failed: %+v", cfg.Paths.Mappings)
	}
	if cfg.Cleanup.SeedTime != 48*time.Hour {
		t.Errorf("seed time override failed: %v", cfg.Cleanup.SeedTime)
	}
	if cfg.Cleanup.MinProgressForCancel != 0.1 {
		t.Errorf("min progress for cancel override failed: %v", cfg.Cleanup.MinProgressForCancel)
	}
}

func TestValidateCollectsAllErrors(t *testing.T) {
	cfg := Config{} // everything empty/zero
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	msg := err.Error()
	for _, want := range []string{"server.listen", "server.external_url", "qbittorrent.url", "stream.wait_timeout"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error mentioning %q, got: %s", want, msg)
		}
	}
}

func TestValidateCleanup(t *testing.T) {
	cfg := Default()
	cfg.Cleanup.SeedTime = -time.Hour
	cfg.Cleanup.MinProgressForCancel = 1.5
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	msg := err.Error()
	for _, want := range []string{"cleanup.seed_time", "cleanup.min_progress_for_cancel"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error mentioning %q, got: %s", want, msg)
		}
	}
}

func TestValidateAllowsZeroSeedTimeToDisableCleanup(t *testing.T) {
	cfg := Default()
	cfg.Cleanup.SeedTime = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected seed_time=0 to be valid (disables cleanup), got %v", err)
	}
}

func TestValidateExternalURLScheme(t *testing.T) {
	cfg := Default()
	cfg.Server.ExternalURL = "ftp://nope"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "external_url") {
		t.Errorf("expected external_url scheme error, got %v", err)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Default()
	cfg.Server.AdminPassword = "secret-pass"
	if err := Save(cfg, path); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Server.AdminPassword != "secret-pass" {
		t.Errorf("round trip lost admin password: %q", loaded.Server.AdminPassword)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	cfg := Config{}
	if err := Save(cfg, filepath.Join(t.TempDir(), "c.yaml")); err == nil {
		t.Fatal("expected validation error on save")
	}
}

func TestEnsureSecrets(t *testing.T) {
	cfg := Default()
	changed, err := EnsureSecrets(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected changed=true")
	}
	if len(cfg.Server.AdminPassword) != 16 {
		t.Errorf("admin password length %d; want 16", len(cfg.Server.AdminPassword))
	}

	changed, err = EnsureSecrets(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("expected changed=false on second call")
	}
}

func TestGenerateSecretUnique(t *testing.T) {
	a, err := GenerateSecret(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateSecret(32)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two generated secrets are equal")
	}
}
