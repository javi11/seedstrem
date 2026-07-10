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
}

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `
server:
  listen: ":9090"
  external_url: "https://media.example.com"
qbittorrent:
  url: "http://qb:8081"
  username: "u"
  password: "p"
stream:
  wait_timeout: 30s
paths:
  mappings:
    - qbit: /dl
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
	if cfg.QBittorrent.Category != "seedstrem" {
		t.Errorf("default category lost: %q", cfg.QBittorrent.Category)
	}
	if len(cfg.Paths.Mappings) != 1 || cfg.Paths.Mappings[0].Local != "/mnt/dl" {
		t.Errorf("mappings not parsed: %+v", cfg.Paths.Mappings)
	}
}

func TestApplyEnv(t *testing.T) {
	cfg := Default()
	env := map[string]string{
		"SEEDSTREM_QBITTORRENT_URL":     "http://other:9999",
		"SEEDSTREM_STREAM_WAIT_TIMEOUT": "90s",
		"SEEDSTREM_PATHS_MAPPINGS":      "/a:/b,/c:/d",
	}
	applyEnv(&cfg, func(k string) string { return env[k] })

	if cfg.QBittorrent.URL != "http://other:9999" {
		t.Errorf("qbit url override failed: %q", cfg.QBittorrent.URL)
	}
	if cfg.Stream.WaitTimeout != 90*time.Second {
		t.Errorf("wait timeout override failed: %v", cfg.Stream.WaitTimeout)
	}
	if len(cfg.Paths.Mappings) != 2 || cfg.Paths.Mappings[1].QBit != "/c" {
		t.Errorf("mappings override failed: %+v", cfg.Paths.Mappings)
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
