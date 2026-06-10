//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

func TestResolveGroups_KnownUser(t *testing.T) {
	ctx := context.Background()

	groups, err := testResolver.ResolveGroups(ctx, cfg.Google.TestEmail)
	if err != nil {
		t.Fatalf("ResolveGroups(%q) error: %v", cfg.Google.TestEmail, err)
	}

	if len(groups) == 0 {
		t.Fatalf("expected at least one group for %q, got none", cfg.Google.TestEmail)
	}

	t.Logf("resolved %d groups for %q:", len(groups), cfg.Google.TestEmail)
	for _, g := range groups {
		t.Logf("  - %s", g)
	}
}

func TestResolveGroups_UnknownUser(t *testing.T) {
	ctx := context.Background()

	// Derive domain from testEmail.
	parts := strings.SplitN(cfg.Google.TestEmail, "@", 2)
	domain := parts[1]

	groups, err := testResolver.ResolveGroups(ctx, "nonexistent-user-xyz-999@"+domain)
	if err != nil {
		// Google API may return an error for nonexistent users, or an empty list.
		// Both are acceptable — the important thing is no panic.
		t.Logf("ResolveGroups for nonexistent user returned error (acceptable): %v", err)

		return
	}

	if len(groups) != 0 {
		t.Fatalf("expected 0 groups for nonexistent user, got %d", len(groups))
	}

	t.Log("nonexistent user returned empty groups (expected)")
}
