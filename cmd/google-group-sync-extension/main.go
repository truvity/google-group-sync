// Package main is the Lambda Extension entry point for google-group-sync.
// It registers with the Lambda Extensions API, then runs the HTTP server as a sidecar.
// Other Lambda functions include this as a Layer and call http://localhost:9090/groups.
//
// All configuration env vars use the GGS_ prefix to avoid collisions with the host
// Lambda's env vars. See config.ExtensionDefaults() for default values.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/truvity/google-group-sync/pkg/app"
	appconfig "github.com/truvity/google-group-sync/pkg/config"
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
		os.Exit(1) //nolint:gocritic // process terminates
	}

	// Optionally load SA key from Secrets Manager (reads GGS_SA_KEY_SECRET_NAME).
	if err := loadSAKeyFromSecretsManager(ctx, "GGS_"); err != nil {
		logger.ErrorContext(ctx, "failed to load SA key from Secrets Manager", slog.Any("error", err))
		os.Exit(1)
	}

	// Run with extension defaults (port 9090, no health server) and GGS_ prefix.
	defaults := appconfig.ExtensionDefaults()
	if err := app.RunWithOptions(ctx, &defaults, appconfig.Options{Prefix: "GGS_"}); err != nil {
		cancel()
		logger.ErrorContext(ctx, "app error", slog.Any("error", err))
		os.Exit(1)
	}
}

// registerExtension registers this process as a Lambda Extension.
func registerExtension(ctx context.Context, logger *slog.Logger) error {
	runtimeAPI := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if runtimeAPI == "" {
		logger.InfoContext(ctx, "AWS_LAMBDA_RUNTIME_API not set, skipping extension registration (local mode)")
		return nil
	}

	registerURL := fmt.Sprintf("http://%s/2020-01-01/extension/register", runtimeAPI)
	registerBody := strings.NewReader(`{"events":[]}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, registerBody) //nolint:gosec // trusted Lambda runtime env var
	if err != nil {
		return fmt.Errorf("create register request: %w", err)
	}

	req.Header.Set("Lambda-Extension-Name", "google-group-sync")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // trusted URL
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

// loadSAKeyFromSecretsManager loads the SA key from Secrets Manager.
// It reads the secret name from {prefix}SA_KEY_SECRET_NAME and sets
// {prefix}GOOGLE_SA_KEY_JSON so the config loader picks it up.
func loadSAKeyFromSecretsManager(ctx context.Context, prefix string) error {
	secretName := os.Getenv(prefix + "SA_KEY_SECRET_NAME")
	if secretName == "" {
		return nil
	}

	// Skip if SA key is already set directly.
	if os.Getenv(prefix+"GOOGLE_SA_KEY_JSON") != "" {
		return nil
	}

	slog.InfoContext(ctx, "loading SA key from Secrets Manager", slog.String("secret", secretName))

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretName,
	})
	if err != nil {
		return fmt.Errorf("get secret %q: %w", secretName, err)
	}

	var secretValue string
	if out.SecretString != nil {
		secretValue = *out.SecretString
	} else {
		return fmt.Errorf("secret %q has no string value", secretName)
	}

	if !json.Valid([]byte(secretValue)) {
		return fmt.Errorf("secret %q value is not valid JSON", secretName)
	}

	// Set with prefix so the config loader finds it.
	if err := os.Setenv(prefix+"GOOGLE_SA_KEY_JSON", secretValue); err != nil {
		return fmt.Errorf("set %sGOOGLE_SA_KEY_JSON: %w", prefix, err)
	}

	slog.InfoContext(ctx, "SA key loaded from Secrets Manager")

	return nil
}
