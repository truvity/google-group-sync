// Package app wires all components and runs the service.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/truvity/google-group-sync/pkg/cache"
	"github.com/truvity/google-group-sync/pkg/config"
	"github.com/truvity/google-group-sync/pkg/resolver"
	"github.com/truvity/google-group-sync/pkg/server"
)

// Run initializes all components and starts the HTTP server.
// It blocks until the context is canceled (signal received).
// Uses DefaultConfig with no prefix (standalone/Lambda mode).
func Run(ctx context.Context) error {
	defaults := config.DefaultConfig()
	return RunWithOptions(ctx, &defaults, config.Options{})
}

// RunWithOptions initializes all components with the given config defaults and options.
func RunWithOptions(ctx context.Context, defaults *config.Config, opts config.Options) error {
	cfg, err := config.Load(defaults, opts)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.LogLevel, cfg.LogFormat)

	saKeys, err := cfg.SAKeySource(logger)
	if err != nil {
		return fmt.Errorf("load SA key: %w", err)
	}

	googleResolver := resolver.NewGoogleResolver(
		logger,
		saKeys,
		cfg.GoogleAdminEmail,
	)

	groupCache, err := cache.New(cfg.CacheMaxSize, cfg.CacheTTL)
	if err != nil {
		return fmt.Errorf("create cache: %w", err)
	}

	// Wrap with cache + singleflight deduplication.
	cachedResolver := resolver.NewCachedResolver(logger, googleResolver, groupCache)

	logger.InfoContext(ctx, "starting google-group-sync",
		slog.Int("port", cfg.Port),
		slog.Int("health_port", cfg.HealthPort),
		slog.String("cache_ttl", cfg.CacheTTL.String()),
		slog.Int("cache_max_size", cfg.CacheMaxSize),
	)

	return server.Run(ctx, logger, server.Config{
		Port:       cfg.Port,
		HealthPort: cfg.HealthPort,
	}, cachedResolver)
}

func newLogger(level, format string) *slog.Logger {
	var lvl slog.Level

	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
