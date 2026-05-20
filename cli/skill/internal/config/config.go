// Package config loads the CLI's runtime configuration. The CLI reads
// `~/.skillcloud/config.yaml` (which `skill login` writes) and merges
// it with environment variables. Env vars always take precedence so an
// operator can override a saved profile for a single command without
// editing the file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the materialised view of the CLI configuration.
type Config struct {
	// Host is the base URL of the Skill Cloud server, e.g.
	// "http://localhost:8080". No trailing slash.
	Host string `yaml:"host"`
	// APIKey is the bearer token used to authenticate to the server.
	// Stored in plaintext at rest (this is parity with kubectl, gh,
	// etc.); operators who need stronger secret handling can leave
	// the file empty and set SKILLCLOUD_API_KEY at runtime instead.
	APIKey string `yaml:"api_key"`
}

const (
	// DefaultHost is used when neither the config file nor the env
	// supplies a host. It matches the docker-compose dev stack.
	DefaultHost = "http://localhost:8080"

	// FilePerm restricts the config file to the owner since it stores
	// a bearer token.
	FilePerm = 0o600
	// DirPerm restricts the config directory likewise.
	DirPerm = 0o700
)

// ErrMissingAPIKey is returned when a command needs an API key but
// none was supplied via config or env.
var ErrMissingAPIKey = errors.New("missing API key: set $SKILLCLOUD_API_KEY or run `skill login`")

// Load reads ~/.skillcloud/config.yaml (if it exists) and merges env
// overrides. A missing file is not an error — the env-only path is a
// valid first-run configuration for CI / scripts.
func Load() (Config, error) {
	cfg := Config{Host: DefaultHost}

	path, err := defaultPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// not an error — fall through to env-only
	case err != nil:
		return cfg, fmt.Errorf("read %s: %w", path, err)
	default:
		var fromFile Config
		if err := yaml.Unmarshal(data, &fromFile); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
		if fromFile.Host != "" {
			cfg.Host = fromFile.Host
		}
		if fromFile.APIKey != "" {
			cfg.APIKey = fromFile.APIKey
		}
	}

	if v := os.Getenv("SKILLCLOUD_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("SKILLCLOUD_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	return cfg, nil
}

// Save writes the config to ~/.skillcloud/config.yaml with 0600
// permissions so the API key isn't world-readable.
func Save(cfg Config) error {
	path, err := defaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), DirPerm); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, FilePerm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Path returns where the config file is read from / written to. It
// honours SKILLCLOUD_CONFIG (so tests can redirect to a tempdir)
// before falling back to ~/.skillcloud/config.yaml.
func Path() (string, error) {
	return defaultPath()
}

func defaultPath() (string, error) {
	if override := os.Getenv("SKILLCLOUD_CONFIG"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".skillcloud", "config.yaml"), nil
}

// Require returns the config but fails when the API key is missing.
// Commands that talk to the server (push / list / call / ...) use
// this; `init` and `login` bypass it.
func Require() (Config, error) {
	cfg, err := Load()
	if err != nil {
		return cfg, err
	}
	if cfg.APIKey == "" {
		return cfg, ErrMissingAPIKey
	}
	return cfg, nil
}
