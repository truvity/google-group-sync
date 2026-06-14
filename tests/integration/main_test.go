//go:build integration

// Package integration provides end-to-end tests against a real Google Workspace.
//
// Prerequisites:
//   - Google SA key stored in system keyring: service="google-group-sync", key="sa-key"
//     Store:  secret-tool store --label='google-group-sync sa-key' service google-group-sync username sa-key < /path/to/sa-key.json
//     Clean:  rm /path/to/sa-key.json  # delete the key file after storing to keyring
//     Verify: secret-tool lookup service google-group-sync username sa-key | head -c 20
//   - Config file at ~/.config/google-group-sync/config.yaml with domain, adminEmail, customerId
//   - Real Google Workspace with at least one user who belongs to at least one group
//
// Run: go test -tags=integration ./tests/integration/...
package integration

import (
	"log/slog"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"

	"github.com/truvity/google-group-sync/pkg/resolver"
)

type (
	testConfig struct {
		Google struct {
			AdminEmail string `yaml:"adminEmail"`
			// TestEmail is a user email known to have groups (for positive test).
			TestEmail string `yaml:"testEmail"`
		} `yaml:"google"`
	}
)

var (
	testResolver resolver.GroupLister
	cfg          testConfig
)

func TestMain(m *testing.M) {
	// Load config from XDG path.
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("failed to get home dir", slog.Any("error", err))
		os.Exit(1)
	}

	configPath := home + "/.config/google-group-sync/config.yaml"

	data, err := os.ReadFile(configPath)
	if err != nil {
		slog.Error("failed to read config", slog.String("path", configPath), slog.Any("error", err))
		slog.Error("create ~/.config/google-group-sync/config.yaml with google.domain, google.adminEmail, google.customerId, google.testEmail")
		os.Exit(1)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		slog.Error("failed to parse config", slog.Any("error", err))
		os.Exit(1)
	}

	if cfg.Google.TestEmail == "" {
		slog.Error("config.google.testEmail is required (a user known to have groups)")
		os.Exit(1)
	}

	// Load SA key from system keyring.
	saKeyJSON, err := keyring.Get("google-group-sync", "sa-key")
	if err != nil {
		slog.Error("failed to read SA key from keyring",
			slog.Any("error", err),
			slog.String("hint", "store with: go run ./cmd/testsetup store google-group-sync sa-key < /path/to/sa.json"),
		)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testResolver = resolver.NewGoogleResolver(
		logger,
		[]byte(saKeyJSON),
		cfg.Google.AdminEmail,
	)

	os.Exit(m.Run())
}
