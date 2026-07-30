// Package config provides environment-based configuration for google-group-sync.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/truvity/google-group-sync/pkg/keysource"
)

// Config holds all service configuration loaded from environment variables.
type Config struct {
	// Google Workspace settings.
	GoogleAdminEmail string `env:"GOOGLE_ADMIN_EMAIL"`
	GoogleSAKeyJSON  string `env:"GOOGLE_SA_KEY_JSON"`
	GoogleSAKeyFile  string `env:"GOOGLE_SA_KEY_FILE"`

	// Secrets Manager (Lambda: load SA key at startup).
	SAKeySecretName string `env:"SA_KEY_SECRET_NAME"`

	// Server settings.
	Port       int `env:"PORT"`
	HealthPort int `env:"HEALTH_PORT"`

	// Cache settings.
	CacheTTL     time.Duration `env:"CACHE_TTL"`
	CacheMaxSize int           `env:"CACHE_MAX_SIZE"`

	// Logging.
	LogLevel  string `env:"LOG_LEVEL"`
	LogFormat string `env:"LOG_FORMAT"`
}

// Options controls how config is loaded. Different entry points pass different options.
type Options struct {
	// Prefix for env var names (e.g., "GGS_" reads GGS_PORT instead of PORT).
	Prefix string
}

// DefaultConfig returns the standard defaults shared by all entry points.
func DefaultConfig() Config {
	return Config{
		Port:         8080,
		HealthPort:   7070,
		CacheTTL:     5 * time.Minute,
		CacheMaxSize: 10000,
		LogLevel:     "info",
		LogFormat:    "json",
	}
}

// ExtensionDefaults returns defaults for the Lambda Extension entry point.
// Port 9090 (avoids conflict with host Lambda), health disabled.
func ExtensionDefaults() Config {
	cfg := DefaultConfig()
	cfg.Port = 9090
	cfg.HealthPort = 0
	return cfg
}

// Load reads configuration from environment variables with the given options.
// The config starts from the provided defaults, then env vars override.
func Load(defaults *Config, opts Options) (*Config, error) {
	cfg := *defaults

	envOpts := env.Options{}
	if opts.Prefix != "" {
		envOpts.Prefix = opts.Prefix
	}

	if err := env.ParseWithOptions(&cfg, envOpts); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// SAKeyJSON returns the SA key JSON content, reading from file if necessary.
func (c *Config) SAKeyJSON() ([]byte, error) {
	if c.GoogleSAKeyJSON != "" {
		return []byte(c.GoogleSAKeyJSON), nil
	}

	if c.GoogleSAKeyFile == "" {
		return nil, fmt.Errorf("neither GOOGLE_SA_KEY_JSON nor GOOGLE_SA_KEY_FILE is set")
	}

	data, err := os.ReadFile(c.GoogleSAKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read SA key file %q: %w", c.GoogleSAKeyFile, err)
	}

	return data, nil
}

// SAKeySource returns the key source for the resolver: static bytes for
// an env-injected key, a re-reading file source for the Secret mount —
// the latter picks up rotations without a restart.
func (c *Config) SAKeySource(logger *slog.Logger) (keysource.Source, error) {
	if c.GoogleSAKeyJSON != "" {
		return keysource.Static([]byte(c.GoogleSAKeyJSON)), nil
	}

	if c.GoogleSAKeyFile == "" {
		return nil, fmt.Errorf("neither GOOGLE_SA_KEY_JSON nor GOOGLE_SA_KEY_FILE is set")
	}

	// Fail fast on a broken mount at startup; afterwards the source
	// serves last-known-good through transient swap windows.
	if _, err := os.Stat(c.GoogleSAKeyFile); err != nil {
		return nil, fmt.Errorf("SA key file %q: %w", c.GoogleSAKeyFile, err)
	}

	return keysource.File(logger, c.GoogleSAKeyFile), nil
}

func (c *Config) validate() error {
	if c.GoogleAdminEmail == "" {
		return fmt.Errorf("GOOGLE_ADMIN_EMAIL is required")
	}

	if c.GoogleSAKeyJSON == "" && c.GoogleSAKeyFile == "" {
		return fmt.Errorf("either GOOGLE_SA_KEY_JSON or GOOGLE_SA_KEY_FILE is required")
	}

	if c.GoogleSAKeyJSON != "" && c.GoogleSAKeyFile != "" {
		return fmt.Errorf("GOOGLE_SA_KEY_JSON and GOOGLE_SA_KEY_FILE are mutually exclusive")
	}

	return nil
}
