package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/truvity/google-group-sync/pkg/cache"
	"github.com/truvity/google-group-sync/pkg/server"
)

type (
	mockResolver struct {
		groups []string
		err    error
	}
)

func (m *mockResolver) ResolveGroups(_ context.Context, _ string) ([]string, error) {
	return m.groups, m.err
}

func newTestApp(t *testing.T, res *mockResolver) *fiber.App {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	c, err := cache.New(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Post("/groups", server.NewGroupsHandler(logger, res, c))

	return app
}

func TestHandler_Success(t *testing.T) {
	res := &mockResolver{groups: []string{"admins@example.com", "devs@example.com"}}
	app := newTestApp(t, res)

	body := `{"email": "user@example.com"}`
	req := httptest.NewRequest("POST", "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Groups []string `json:"groups"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}
}

func TestHandler_EmptyEmail(t *testing.T) {
	res := &mockResolver{}
	app := newTestApp(t, res)

	body := `{"email": ""}`
	req := httptest.NewRequest("POST", "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var problem struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}

	if problem.Status != 400 {
		t.Fatalf("expected problem status 400, got %d", problem.Status)
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	res := &mockResolver{}
	app := newTestApp(t, res)

	body := `not json`
	req := httptest.NewRequest("POST", "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_GoogleAPIError(t *testing.T) {
	res := &mockResolver{err: errors.New("googleapi: Error 403: Not Authorized")}
	app := newTestApp(t, res)

	body := `{"email": "user@example.com"}`
	req := httptest.NewRequest("POST", "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

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
		Detail string `json:"detail"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}

	if problem.Status != 502 {
		t.Fatalf("expected problem status 502, got %d", problem.Status)
	}
}

func TestHandler_NoGroups(t *testing.T) {
	res := &mockResolver{groups: nil}
	app := newTestApp(t, res)

	body := `{"email": "user@example.com"}`
	req := httptest.NewRequest("POST", "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Groups []string `json:"groups"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	// Must be empty array, not null.
	if result.Groups == nil {
		t.Fatal("expected non-null empty array")
	}

	if len(result.Groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(result.Groups))
	}
}

func TestHandler_CacheHit(t *testing.T) {
	callCount := 0
	res := &mockResolver{groups: []string{"cached@example.com"}}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	c, _ := cache.New(100, 5*time.Minute)

	app := fiber.New()
	app.Post("/groups", server.NewGroupsHandler(logger, &countingResolver{
		inner: res,
		count: &callCount,
	}, c))

	body := `{"email": "user@example.com"}`

	// First call — cache miss.
	req := httptest.NewRequest("POST", "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	_ = resp.Body.Close()

	// Second call — cache hit.
	req = httptest.NewRequest("POST", "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ = app.Test(req)
	_ = resp.Body.Close()

	if callCount != 1 {
		t.Fatalf("expected resolver called once (cache hit on second), got %d calls", callCount)
	}
}

type countingResolver struct {
	inner *mockResolver
	count *int
}

func (c *countingResolver) ResolveGroups(ctx context.Context, email string) ([]string, error) {
	*c.count++

	return c.inner.ResolveGroups(ctx, email)
}
