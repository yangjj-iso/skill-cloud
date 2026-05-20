package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withConfigPath redirects $SKILLCLOUD_CONFIG to a temp file for the
// duration of t and resets every config-related env var so the test
// doesn't inherit ambient state.
func withConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	t.Setenv("SKILLCLOUD_CONFIG", path)
	t.Setenv("SKILLCLOUD_HOST", "")
	t.Setenv("SKILLCLOUD_API_KEY", "")
	return path
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	withConfigPath(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Host != DefaultHost {
		t.Fatalf("host = %q, want default %q", cfg.Host, DefaultHost)
	}
	if cfg.APIKey != "" {
		t.Fatalf("api key should be empty, got %q", cfg.APIKey)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := withConfigPath(t)
	want := Config{Host: "http://example:9000", APIKey: "k-123"}
	if err := Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// 0600 — the config holds a bearer token.
	if info.Mode().Perm() != FilePerm {
		t.Fatalf("perm = %v, want %v", info.Mode().Perm(), FilePerm)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	withConfigPath(t)
	if err := Save(Config{Host: "http://file:8080", APIKey: "from-file"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Setenv("SKILLCLOUD_HOST", "http://env:7070")
	t.Setenv("SKILLCLOUD_API_KEY", "from-env")
	got, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Host != "http://env:7070" {
		t.Fatalf("host should come from env, got %q", got.Host)
	}
	if got.APIKey != "from-env" {
		t.Fatalf("api key should come from env, got %q", got.APIKey)
	}
}

func TestRequireFailsWithoutAPIKey(t *testing.T) {
	withConfigPath(t)
	if _, err := Require(); err == nil {
		t.Fatal("expected Require() to fail when no API key is configured")
	}
}

func TestRequireSucceedsWithEnvAPIKey(t *testing.T) {
	withConfigPath(t)
	t.Setenv("SKILLCLOUD_API_KEY", "k")
	cfg, err := Require()
	if err != nil {
		t.Fatalf("require: %v", err)
	}
	if cfg.APIKey != "k" {
		t.Fatalf("api key = %q", cfg.APIKey)
	}
}
