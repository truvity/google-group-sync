//go:build integration

package integration

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/truvity/google-group-sync/pkg/cache"
	"github.com/truvity/google-group-sync/pkg/resolver"
)

// TestCachedResolver_Integration verifies that the CachedResolver properly caches
// results from the real Google API — the second call should be instant (cache hit).
func TestCachedResolver_Integration(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	c, err := cache.NewMemoryCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	cached := resolver.NewCachedResolver(logger, testResolver, c)

	// First call — cache miss, hits Google API.
	start := time.Now()

	groups1, err := cached.ResolveGroups(ctx, cfg.Google.TestEmail)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	firstDuration := time.Since(start)

	if len(groups1) == 0 {
		t.Fatalf("expected groups for %q, got none", cfg.Google.TestEmail)
	}

	t.Logf("first call (cache miss): %d groups in %s", len(groups1), firstDuration)

	// Second call — cache hit, should be nearly instant.
	start = time.Now()

	groups2, err := cached.ResolveGroups(ctx, cfg.Google.TestEmail)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	secondDuration := time.Since(start)

	if len(groups2) != len(groups1) {
		t.Errorf("cache returned different group count: first=%d, second=%d", len(groups1), len(groups2))
	}

	// Cache hit should be orders of magnitude faster than the API call.
	if secondDuration > 1*time.Millisecond {
		t.Errorf("cache hit took too long (%s), expected sub-millisecond", secondDuration)
	}

	t.Logf("second call (cache hit): %d groups in %s", len(groups2), secondDuration)
}

// TestCachedResolver_InterfaceContract verifies the Cache interface works
// correctly when injected into the CachedResolver with a real backend.
func TestCachedResolver_InterfaceContract(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Use the interface type explicitly to verify contract.
	var c cache.Cache
	mc, err := cache.NewMemoryCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	c = mc

	cached := resolver.NewCachedResolver(logger, testResolver, c)

	groups, err := cached.ResolveGroups(ctx, cfg.Google.TestEmail)
	if err != nil {
		t.Fatalf("ResolveGroups via interface: %v", err)
	}

	if len(groups) == 0 {
		t.Fatal("expected at least one group via interface-based cache")
	}

	// Verify it's in the cache now.
	cachedGroups, ok := c.Get(cfg.Google.TestEmail)
	if !ok {
		t.Fatal("expected cache entry after ResolveGroups")
	}

	if len(cachedGroups) != len(groups) {
		t.Errorf("cache entry count mismatch: resolved=%d, cached=%d", len(groups), len(cachedGroups))
	}

	t.Logf("interface contract verified: %d groups cached", len(cachedGroups))
}
