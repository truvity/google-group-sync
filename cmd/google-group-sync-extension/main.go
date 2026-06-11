// Package main is the Lambda Extension entry point for google-group-sync.
// It registers with the Lambda Extensions API, then runs the HTTP server as a sidecar.
// Other Lambda functions include this as a Layer and call http://localhost:9090/groups.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/truvity/google-group-sync/pkg/app"
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Register with Lambda Extensions API.
	if err := registerExtension(ctx, logger); err != nil {
		logger.ErrorContext(ctx, "failed to register extension", slog.Any("error", err))
		os.Exit(1) //nolint:gocritic // cancel() called by os.Exit doesn't matter here — process terminates
	}

	// Set default port to 9090 if not configured (different from main function's 8080).
	if os.Getenv("PORT") == "" {
		if err := os.Setenv("PORT", "9090"); err != nil {
			logger.ErrorContext(ctx, "failed to set PORT", slog.Any("error", err))
			os.Exit(1)
		}
	}

	// Run the HTTP server (blocks until context canceled).
	if err := app.Run(ctx); err != nil {
		cancel()
		logger.ErrorContext(ctx, "app error", slog.Any("error", err))
		os.Exit(1)
	}
}

// registerExtension registers this process as a Lambda Extension.
// https://docs.aws.amazon.com/lambda/latest/dg/runtimes-extensions-api.html
func registerExtension(ctx context.Context, logger *slog.Logger) error {
	runtimeAPI := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if runtimeAPI == "" {
		logger.InfoContext(ctx, "AWS_LAMBDA_RUNTIME_API not set, skipping extension registration (local mode)")
		return nil
	}

	registerURL := fmt.Sprintf("http://%s/2020-01-01/extension/register", runtimeAPI)

	// Extensions API requires a JSON body with events array (empty = no lifecycle subscriptions).
	registerBody := strings.NewReader(`{"events":[]}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, registerBody) //nolint:gosec // URL built from trusted Lambda runtime env var
	if err != nil {
		return fmt.Errorf("create register request: %w", err)
	}

	// The extension name must match the binary filename in /opt/extensions/.
	req.Header.Set("Lambda-Extension-Name", "google-group-sync")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // req is constructed above with trusted URL
	if err != nil {
		return fmt.Errorf("register extension: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register extension: status %d: %s", resp.StatusCode, string(body))
	}

	extensionID := resp.Header.Get("Lambda-Extension-Identifier")
	logger.InfoContext(ctx, "extension registered", slog.String("extension_id", extensionID))

	return nil
}
