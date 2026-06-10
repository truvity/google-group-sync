// Package config provides environment-based configuration for google-group-sync.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all service configuration loaded from environment variables.
type Config struct {
	// Google Workspace settings.
	GoogleAdminEmail string
	GoogleSAKeyJSON  string // Raw SA key JSON (mutual exclusive with SAKeyFile).
	GoogleSAKeyFile  string // Path to SA key file (mutual exclusive with SAKeyJSON).

	// Server settings.
	Port       int
	HealthPort int

	// Cache settings.
	CacheTTL     time.Duration
	CacheMaxSize int

	// Logging.
	LogLevel  string
	LogFormat string
}

// Load reads configuration from environment variables and validates it.
func Load() (*Config, error) {
	cfg := &Config{
		GoogleAdminEmail: os.Getenv("GOOGLE_ADMIN_EMAIL"),
		GoogleSAKeyJSON:  os.Getenv("GOOGLE_SA_KEY_JSON"),
		GoogleSAKeyFile:  os.Getenv("GOOGLE_SA_KEY_FILE"),
		LogLevel:         envOrDefault("LOG_LEVEL", "info"),
		LogFormat:        envOrDefault("LOG_FORMAT", "json"),
	}

	var err error

	cfg.Port, err = envIntOrDefault("PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}

	cfg.HealthPort, err = envIntOrDefault("HEALTH_PORT", 7070)
	if err != nil {
		return nil, fmt.Errorf("invalid HEALTH_PORT: %w", err)
	}

	cfg.CacheTTL, err = envDurationOrDefault("CACHE_TTL", 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("invalid CACHE_TTL: %w", err)
	}

	cfg.CacheMaxSize, err = envIntOrDefault("CACHE_MAX_SIZE", 10000)
	if err != nil {
		return nil, fmt.Errorf("invalid CACHE_MAX_SIZE: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SAKeyJSON returns the SA key JSON content, reading from file if necessary.
func (c *Config) SAKeyJSON() ([]byte, error) {
	if c.GoogleSAKeyJSON != "" {
		return []byte(c.GoogleSAKeyJSON), nil
	}

	data, err := os.ReadFile(c.GoogleSAKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read SA key file %q: %w", c.GoogleSAKeyFile, err)
	}

	return data, nil
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

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return defaultVal
}

func envIntOrDefault(key string, defaultVal int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("parse %q=%q: %w", key, v, err)
	}

	return n, nil
}

func envDurationOrDefault(key string, defaultVal time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("parse %q=%q: %w", key, v, err)
	}

	return d, nil
}
