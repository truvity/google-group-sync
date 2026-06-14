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

func TestListGroups(t *testing.T) {
	ctx := context.Background()

	groups, err := testResolver.ListGroups(ctx)
	if err != nil {
		t.Fatalf("ListGroups() error: %v", err)
	}

	if len(groups) == 0 {
		t.Fatal("expected at least one group, got none")
	}

	t.Logf("listed %d groups", len(groups))

	// Verify each group has an email and at least has the structure right.
	for i, g := range groups {
		if g.Email == "" {
			t.Errorf("group[%d] has empty email", i)
		}

		if i < 3 {
			t.Logf("  - %s (%d members)", g.Email, len(g.Members))
		}
	}
}

func TestGetGroup_KnownGroup(t *testing.T) {
	ctx := context.Background()

	// First get the user's groups to find a known group email.
	userGroups, err := testResolver.ResolveGroups(ctx, cfg.Google.TestEmail)
	if err != nil {
		t.Fatalf("ResolveGroups setup: %v", err)
	}

	if len(userGroups) == 0 {
		t.Skip("no groups found for test user, cannot test GetGroup")
	}

	groupEmail := userGroups[0]

	group, err := testResolver.GetGroup(ctx, groupEmail)
	if err != nil {
		t.Fatalf("GetGroup(%q) error: %v", groupEmail, err)
	}

	if group == nil {
		t.Fatalf("GetGroup(%q) returned nil", groupEmail)
	}

	if group.Email != groupEmail {
		t.Errorf("GetGroup returned email %q, want %q", group.Email, groupEmail)
	}

	// The test user should be a member of this group.
	found := false
	for _, member := range group.Members {
		if member == cfg.Google.TestEmail {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected %q to be a member of %q, but not found in %v", cfg.Google.TestEmail, groupEmail, group.Members)
	}

	t.Logf("group %q has %d members, test user is member: %v", groupEmail, len(group.Members), found)
}

func TestGetGroup_NonexistentGroup(t *testing.T) {
	ctx := context.Background()

	group, err := testResolver.GetGroup(ctx, "nonexistent-group-xyz-999@truvity.com")
	if err == nil && group != nil {
		t.Fatal("expected error or nil for nonexistent group")
	}

	if err != nil {
		t.Logf("nonexistent group returned error (expected): %v", err)
	}
}
