package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// Suspended marks emails whose account the directory reports
	// suspended.
	suspended map[string]bool

	// Call counting.
	resolveCallCount int
}

func (m *mockGroupLister) ResolveGroups(ctx context.Context, email string) ([]string, error) {
	ug, err := m.ResolveUser(ctx, email)
	if err != nil {
		return nil, err
	}

	return ug.Groups, nil
}

func (m *mockGroupLister) ResolveUser(_ context.Context, email string) (resolver.UserGroups, error) {
	m.resolveCallCount++

	if m.resolveErr != nil {
		return resolver.UserGroups{}, m.resolveErr
	}

	ug := resolver.UserGroups{Groups: []string{}, Suspended: m.suspended[email]}

	if groups, ok := m.userGroups[email]; ok {
		ug.Groups = groups
	}

	return ug, nil
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

// The suspension signal reaches the wire: a consumer deciding grants
// must be able to tell "suspended, revoke" from the ambiguous "no
// groups" — and an active account must never read as suspended.
func TestUserGroups_SuspendedSignal(t *testing.T) {
	mock := &mockGroupLister{
		userGroups: map[string][]string{
			"gone@example.com": {"admins@example.com"},
			"live@example.com": {"admins@example.com"},
		},
		suspended: map[string]bool{"gone@example.com": true},
	}
	app := newTestApp(t, mock)

	for email, want := range map[string]bool{"gone@example.com": true, "live@example.com": false} {
		req := httptest.NewRequest("GET", "/users/"+email+"/groups", http.NoBody)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}

		var result struct {
			Suspended bool `json:"suspended"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}

		_ = resp.Body.Close()

		if result.Suspended != want {
			t.Fatalf("%s: expected suspended=%v, got %v", email, want, result.Suspended)
		}
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

// newLoggedUserGroupsApp builds an app with only the user-groups route,
// logging JSON at DEBUG level into the returned buffer.
func newLoggedUserGroupsApp(t *testing.T, res resolver.GroupLister) (*fiber.App, *bytes.Buffer) {
	t.Helper()

	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := fiber.New()
	app.Get("/users/:email/groups", server.NewUserGroupsHandler(logger, res))

	return app, buf
}

func TestUserGroups_LogsLookupAtInfo(t *testing.T) {
	mock := &mockGroupLister{
		userGroups: map[string][]string{
			"user@example.com": {"admins@example.com", "devs@example.com"},
		},
	}
	app, buf := newLoggedUserGroupsApp(t, mock)

	req := httptest.NewRequest("GET", "/users/user@example.com/groups", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	logs := buf.String()

	for _, want := range []string{
		`"level":"INFO"`,
		`"msg":"resolved user groups"`,
		`"email":"user@example.com"`,
		`"groups":2`,
		`"cached":false`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("expected log output to contain %s, got:\n%s", want, logs)
		}
	}

	// Group names appear only in the DEBUG detail line.
	if !strings.Contains(logs, `"msg":"user groups detail"`) {
		t.Errorf("expected DEBUG detail line with group names, got:\n%s", logs)
	}
}

func TestUserGroups_LogsWarnOnZeroGroups(t *testing.T) {
	mock := &mockGroupLister{userGroups: map[string][]string{}}
	app, buf := newLoggedUserGroupsApp(t, mock)

	req := httptest.NewRequest("GET", "/users/nobody@example.com/groups", http.NoBody)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	logs := buf.String()

	for _, want := range []string{
		`"level":"WARN"`,
		`"msg":"user groups lookup returned no groups"`,
		`"email":"nobody@example.com"`,
		`"groups":0`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("expected log output to contain %s, got:\n%s", want, logs)
		}
	}
}

func TestUserGroups_LogsCachedFlag(t *testing.T) {
	mock := &mockGroupLister{
		userGroups: map[string][]string{
			"user@example.com": {"admins@example.com"},
		},
	}

	c, err := cache.New(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	cached := resolver.NewCachedResolver(discard, mock, c)

	app, buf := newLoggedUserGroupsApp(t, cached)

	// First request — cache miss.
	resp, err := app.Test(httptest.NewRequest("GET", "/users/user@example.com/groups", http.NoBody))
	if err != nil {
		t.Fatal(err)
	}

	_ = resp.Body.Close()

	if !strings.Contains(buf.String(), `"cached":false`) {
		t.Errorf("expected first lookup logged as cached=false, got:\n%s", buf.String())
	}

	buf.Reset()

	// Second request — served from cache.
	resp, err = app.Test(httptest.NewRequest("GET", "/users/user@example.com/groups", http.NoBody))
	if err != nil {
		t.Fatal(err)
	}

	_ = resp.Body.Close()

	if !strings.Contains(buf.String(), `"cached":true`) {
		t.Errorf("expected second lookup logged as cached=true, got:\n%s", buf.String())
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

func (c *countingGroupLister) ResolveUser(ctx context.Context, email string) (resolver.UserGroups, error) {
	*c.count++

	return c.inner.ResolveUser(ctx, email)
}

func (c *countingGroupLister) ListGroups(ctx context.Context) ([]resolver.Group, error) {
	*c.count++

	return c.inner.ListGroups(ctx)
}

func (c *countingGroupLister) GetGroup(ctx context.Context, groupEmail string) (*resolver.Group, error) {
	*c.count++

	return c.inner.GetGroup(ctx, groupEmail)
}
