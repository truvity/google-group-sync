package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/truvity/google-group-sync/pkg/cache"
	"github.com/truvity/google-group-sync/pkg/resolver"
	"github.com/truvity/google-group-sync/pkg/server"
)

type mockGroupLister struct {
	// ResolveGroups behavior.
	userGroups map[string][]string
	resolveErr error

	// ListGroups behavior.
	allGroups []resolver.Group
	listErr   error

	// GetGroup behavior.
	groupMap map[string]*resolver.Group
	getErr   error

	// Call counting.
	resolveCallCount int
}

func (m *mockGroupLister) ResolveGroups(_ context.Context, email string) ([]string, error) {
	m.resolveCallCount++

	if m.resolveErr != nil {
		return nil, m.resolveErr
	}

	if groups, ok := m.userGroups[email]; ok {
		return groups, nil
	}

	return []string{}, nil
}

func (m *mockGroupLister) ListGroups(_ context.Context) ([]resolver.Group, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	return m.allGroups, nil
}

func (m *mockGroupLister) GetGroup(_ context.Context, groupEmail string) (*resolver.Group, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}

	if group, ok := m.groupMap[groupEmail]; ok {
		return group, nil
	}

	return nil, errors.New("not found")
}

func newTestApp(t *testing.T, mock *mockGroupLister) *fiber.App {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	app := fiber.New()
	app.Get("/groups/:email", server.NewGetGroupHandler(logger, mock))
	app.Get("/groups", server.NewListGroupsHandler(logger, mock))
	app.Get("/users/:email/groups", server.NewUserGroupsHandler(logger, mock))

	return app
}

func TestListGroups_Success(t *testing.T) {
	mock := &mockGroupLister{
		allGroups: []resolver.Group{
			{Email: "eng-admin@example.com", Members: []string{"user1@example.com", "user2@example.com"}},
			{Email: "devs@example.com", Members: []string{"user1@example.com"}},
		},
	}
	app := newTestApp(t, mock)

	req := httptest.NewRequest("GET", "/groups", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []resolver.Group
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result))
	}

	if result[0].Email != "eng-admin@example.com" {
		t.Fatalf("expected eng-admin@example.com, got %s", result[0].Email)
	}

	if len(result[0].Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(result[0].Members))
	}
}

func TestListGroups_Empty(t *testing.T) {
	mock := &mockGroupLister{allGroups: nil}
	app := newTestApp(t, mock)

	req := httptest.NewRequest("GET", "/groups", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []resolver.Group
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result == nil {
		t.Fatal("expected non-null empty array")
	}

	if len(result) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(result))
	}
}

func TestListGroups_BackwardCompat_RedirectOnEmailParam(t *testing.T) {
	mock := &mockGroupLister{}
	app := newTestApp(t, mock)

	req := httptest.NewRequest("GET", "/groups?email=user@example.com", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Fiber returns 302 for redirects by default with Redirect().To().
	if resp.StatusCode != 302 {
		t.Fatalf("expected 302 redirect, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	expected := "/users/user@example.com/groups"

	if location != expected {
		t.Fatalf("expected redirect to %q, got %q", expected, location)
	}
}

func TestGetGroup_Success(t *testing.T) {
	mock := &mockGroupLister{
		groupMap: map[string]*resolver.Group{
			"eng-admin@example.com": {
				Email:   "eng-admin@example.com",
				Members: []string{"user1@example.com", "user2@example.com"},
			},
		},
	}
	app := newTestApp(t, mock)

	req := httptest.NewRequest("GET", "/groups/eng-admin@example.com", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Email   string   `json:"email"`
		Members []string `json:"members"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Email != "eng-admin@example.com" {
		t.Fatalf("expected eng-admin@example.com, got %s", result.Email)
	}

	if len(result.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(result.Members))
	}
}

func TestGetGroup_NotFound(t *testing.T) {
	mock := &mockGroupLister{
		groupMap: map[string]*resolver.Group{},
		getErr:   errors.New("not found"),
	}
	app := newTestApp(t, mock)

	req := httptest.NewRequest("GET", "/groups/unknown@example.com", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Non-Google error → 502 (we can't detect if it's a 4xx without googleapi.Error).
	if resp.StatusCode != 502 {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
}

func TestUserGroups_Success(t *testing.T) {
	mock := &mockGroupLister{
		userGroups: map[string][]string{
			"user@example.com": {"admins@example.com", "devs@example.com"},
		},
	}
	app := newTestApp(t, mock)

	req := httptest.NewRequest("GET", "/users/user@example.com/groups", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Email != "user@example.com" {
		t.Fatalf("expected user@example.com, got %s", result.Email)
	}

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}
}

func TestUserGroups_GoogleAPIError(t *testing.T) {
	mock := &mockGroupLister{
		resolveErr: errors.New("googleapi: Error 403: Not Authorized"),
	}
	app := newTestApp(t, mock)

	req := httptest.NewRequest("GET", "/users/user@example.com/groups", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 502 {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}

	var problem struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}

	if problem.Status != 502 {
		t.Fatalf("expected problem status 502, got %d", problem.Status)
	}
}

func TestUserGroups_NoGroups(t *testing.T) {
	mock := &mockGroupLister{
		userGroups: map[string][]string{},
	}
	app := newTestApp(t, mock)

	req := httptest.NewRequest("GET", "/users/user@example.com/groups", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Groups == nil {
		t.Fatal("expected non-null empty array")
	}

	if len(result.Groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(result.Groups))
	}
}

func TestCachedResolver_Singleflight(t *testing.T) {
	// Verify that the CachedResolver properly uses cache.
	callCount := 0
	inner := &mockGroupLister{
		userGroups: map[string][]string{
			"user@example.com": {"cached@example.com"},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	c, err := cache.New(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	counted := &countingGroupLister{inner: inner, count: &callCount}
	cached := resolver.NewCachedResolver(logger, counted, c)

	ctx := context.Background()

	// First call — cache miss.
	groups, err := cached.ResolveGroups(ctx, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 1 || groups[0] != "cached@example.com" {
		t.Fatalf("unexpected groups: %v", groups)
	}

	// Second call — cache hit (should not call inner).
	groups, err = cached.ResolveGroups(ctx, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 1 {
		t.Fatalf("expected 1 group on cache hit, got %d", len(groups))
	}

	if callCount != 1 {
		t.Fatalf("expected inner called once (cache hit on second), got %d calls", callCount)
	}
}

type countingGroupLister struct {
	inner *mockGroupLister
	count *int
}

func (c *countingGroupLister) ResolveGroups(ctx context.Context, email string) ([]string, error) {
	*c.count++

	return c.inner.ResolveGroups(ctx, email)
}

func (c *countingGroupLister) ListGroups(ctx context.Context) ([]resolver.Group, error) {
	*c.count++

	return c.inner.ListGroups(ctx)
}

func (c *countingGroupLister) GetGroup(ctx context.Context, groupEmail string) (*resolver.Group, error) {
	*c.count++

	return c.inner.GetGroup(ctx, groupEmail)
}
