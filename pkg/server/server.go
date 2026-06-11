package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	slogfiber "github.com/samber/slog-fiber"

	"github.com/truvity/google-group-sync/pkg/cache"
	"github.com/truvity/google-group-sync/pkg/resolver"
)

// Config holds server configuration.
type Config struct {
	Port       int
	HealthPort int
}

// Run starts the HTTP server and blocks until the context is canceled.
func Run(ctx context.Context, logger *slog.Logger, cfg Config, res resolver.GroupResolver, groupCache *cache.Cache) error {
	app := fiber.New(fiber.Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	})

	// Request logging middleware.
	app.Use(slogfiber.New(logger))

	// Routes.
	app.Post("/groups", NewGroupsHandler(logger, res, groupCache))
	app.Get("/health", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	// Health probe on separate port (for Lambda Web Adapter readiness check).
	// Skip if port is 0 (extension mode — no health check needed).
	if cfg.HealthPort > 0 {
		healthApp := fiber.New()
		healthApp.Get("/health", func(c fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		go func() {
			addr := fmt.Sprintf(":%d", cfg.HealthPort)
			if err := healthApp.Listen(addr); err != nil {
				logger.ErrorContext(ctx, "health server error", slog.Any("error", err))
			}
		}()

		defer func() { _ = healthApp.ShutdownWithTimeout(1 * time.Second) }()
	}

	// Start main server in background.
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Port)
		logger.InfoContext(ctx, "listening", slog.String("addr", addr))

		if err := app.Listen(addr); err != nil {
			logger.ErrorContext(ctx, "server error", slog.Any("error", err))
		}
	}()

	// Block until context is canceled (signal from parent).
	<-ctx.Done()
	logger.InfoContext(ctx, "shutting down")

	if err := app.ShutdownWithTimeout(5 * time.Second); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return nil
}
