// Package main is the Lambda entry point for google-group-sync.
// Binary name: bootstrap (Lambda runtime requirement).
// Uses LWA layer for event→HTTP translation.
// Optionally loads SA key from AWS Secrets Manager via SA_KEY_SECRET_NAME env var.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/truvity/google-group-sync/pkg/app"
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Optionally load SA key from Secrets Manager before starting the app.
	if err := loadSAKeyFromSecretsManager(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: load SA key from Secrets Manager: %v\n", err)
		os.Exit(1) //nolint:gocritic // cancel() deferred but process terminates — acceptable
	}

	if err := app.Run(ctx); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
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
