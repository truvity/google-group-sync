// Package main is the entry point for google-group-sync.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/truvity/google-group-sync/pkg/app"
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("google-group-sync %s\n", Version)
			return
		case "--help", "-h":
			printHelp()
			return
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := app.Run(ctx); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: cancel() called explicitly above
	}
}

func printHelp() {
	fmt.Print(`google-group-sync — Google Workspace group membership resolver

Usage: google-group-sync [--version] [--help]

Starts an HTTP server that resolves group memberships for a user email
via the Google Admin SDK Directory API.

Environment variables:
  GOOGLE_ADMIN_EMAIL    Admin email for domain-wide delegation (required)
  GOOGLE_SA_KEY_JSON    Raw SA key JSON (mutually exclusive with SA_KEY_FILE)
  GOOGLE_SA_KEY_FILE    Path to SA key file (mutually exclusive with SA_KEY_JSON)
  PORT                  HTTP server port (default: 8080)
  HEALTH_PORT           Health probe port (default: 7070)
  CACHE_TTL             Cache TTL duration (default: 5m)
  CACHE_MAX_SIZE        Max cache entries (default: 10000)
  LOG_LEVEL             Log level: debug|info|warn|error (default: info)
  LOG_FORMAT            Log format: json|text (default: json)

Deployment:
  Kubernetes    Deploy via Helm chart, mount SA key as Secret volume
  AWS Lambda    Deploy ZIP with bootstrap script + Lambda Web Adapter (LWA) layer
                Auth handled by Function URL (AWS_IAM) or API Gateway

API:
  POST /groups  Resolve group memberships (JSON body: {"email": "user@example.com"})
  GET  /health  Health check (200 OK)
`)
}
