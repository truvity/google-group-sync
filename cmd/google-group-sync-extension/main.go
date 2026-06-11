// Package main is the Lambda Extension entry point for google-group-sync.
// It registers with the Lambda Extensions API, then runs the HTTP server as a sidecar.
// Other Lambda functions include this as a Layer and call http://localhost:9090/groups.
//
// All configuration env vars use the GGS_ prefix to avoid collisions with the host
// Lambda's env vars (e.g., GGS_PORT instead of PORT, GGS_GOOGLE_ADMIN_EMAIL instead
// of GOOGLE_ADMIN_EMAIL). The extension maps these to the standard names before
// starting the app.
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
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"
)

// envMapping defines the GGS_ prefixed env vars and their standard equivalents.
// The extension reads GGS_* and sets the standard names for pkg/config to consume.
var envMapping = []struct {
	ggsName      string
	standardName string
	defaultValue string
}{
	{"GGS_PORT", "PORT", "9090"},
	{"GGS_HEALTH_PORT", "HEALTH_PORT", "7070"},
	{"GGS_GOOGLE_ADMIN_EMAIL", "GOOGLE_ADMIN_EMAIL", ""},
	{"GGS_GOOGLE_SA_KEY_JSON", "GOOGLE_SA_KEY_JSON", ""},
	{"GGS_GOOGLE_SA_KEY_FILE", "GOOGLE_SA_KEY_FILE", ""},
	{"GGS_SA_KEY_SECRET_NAME", "SA_KEY_SECRET_NAME", ""},
	{"GGS_CACHE_TTL", "CACHE_TTL", ""},
	{"GGS_CACHE_MAX_SIZE", "CACHE_MAX_SIZE", ""},
	{"GGS_LOG_LEVEL", "LOG_LEVEL", ""},
	{"GGS_LOG_FORMAT", "LOG_FORMAT", ""},
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Register with Lambda Extensions API.
	if err := registerExtension(ctx, logger); err != nil {
		logger.ErrorContext(ctx, "failed to register extension", slog.Any("error", err))
		os.Exit(1) //nolint:gocritic // cancel() called by os.Exit doesn't matter here — process terminates
	}

	// Map GGS_ prefixed env vars to standard names for pkg/config.
	mapEnvVars()

	// Optionally load SA key from Secrets Manager before starting the app.
	if err := loadSAKeyFromSecretsManager(ctx); err != nil {
		logger.ErrorContext(ctx, "failed to load SA key from Secrets Manager", slog.Any("error", err))
		os.Exit(1)
	}

	// Run the HTTP server (blocks until context canceled).
	if err := app.Run(ctx); err != nil {
		cancel()
		logger.ErrorContext(ctx, "app error", slog.Any("error", err))
		os.Exit(1)
	}
}

// mapEnvVars reads GGS_* env vars and sets the corresponding standard names.
// If a GGS_ var is set, it overrides the standard name. If neither is set,
// the default value is applied (if non-empty).
func mapEnvVars() {
	for _, m := range envMapping {
		if v := os.Getenv(m.ggsName); v != "" {
			_ = os.Setenv(m.standardName, v)
		} else if os.Getenv(m.standardName) == "" && m.defaultValue != "" {
			_ = os.Setenv(m.standardName, m.defaultValue)
		}
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

// loadSAKeyFromSecretsManager loads the SA key from Secrets Manager if SA_KEY_SECRET_NAME is set.
// It sets GOOGLE_SA_KEY_JSON env var so the config package picks it up.
func loadSAKeyFromSecretsManager(ctx context.Context) error {
	secretName := os.Getenv("SA_KEY_SECRET_NAME")
	if secretName == "" {
		return nil // No secret name configured — SA key comes from env directly.
	}

	// Skip if GOOGLE_SA_KEY_JSON is already set (explicit env takes precedence).
	if os.Getenv("GOOGLE_SA_KEY_JSON") != "" {
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
		return fmt.Errorf("secret %q has no string value (binary secrets not supported)", secretName)
	}

	// Validate it looks like JSON before setting.
	if !json.Valid([]byte(secretValue)) {
		return fmt.Errorf("secret %q value is not valid JSON", secretName)
	}

	if err := os.Setenv("GOOGLE_SA_KEY_JSON", secretValue); err != nil {
		return fmt.Errorf("set GOOGLE_SA_KEY_JSON: %w", err)
	}

	slog.InfoContext(ctx, "SA key loaded from Secrets Manager")

	return nil
}
