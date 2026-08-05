package cache_test

import (
	"testing"
	"time"

	"github.com/truvity/google-group-sync/pkg/cache"
)

func ug(groups ...string) cache.UserGroups {
	return cache.UserGroups{Groups: groups}
}

func TestMemoryCache_SetAndGet(t *testing.T) {
	c, err := cache.NewMemoryCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("user@example.com", ug("group-a@example.com", "group-b@example.com"))

	got, ok := c.Get("user@example.com")
	if !ok {
		t.Fatal("expected cache hit")
	}

	if len(got.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(got.Groups))
	}

	if got.Groups[0] != "group-a@example.com" {
		t.Fatalf("expected group-a@example.com, got %s", got.Groups[0])
	}
}

// The suspension signal must survive the cache round-trip: a cached
// suspended answer that comes back as active would re-grant on every
// warm read.
func TestMemoryCache_SuspendedRoundTrip(t *testing.T) {
	c, err := cache.NewMemoryCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("gone@example.com", cache.UserGroups{Groups: []string{"g1"}, Suspended: true})

	got, ok := c.Get("gone@example.com")
	if !ok {
		t.Fatal("expected cache hit")
	}

	if !got.Suspended {
		t.Fatal("suspension signal lost in the cache")
	}
}

func TestMemoryCache_Miss(t *testing.T) {
	c, err := cache.NewMemoryCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := c.Get("unknown@example.com")
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestMemoryCache_Expiry(t *testing.T) {
	c, err := cache.NewMemoryCache(100, 1*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("user@example.com", ug("group@example.com"))

	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("user@example.com")
	if ok {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestMemoryCache_LRUEviction(t *testing.T) {
	c, err := cache.NewMemoryCache(2, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("a@example.com", ug("g1"))
	c.Set("b@example.com", ug("g2"))
	c.Set("c@example.com", ug("g3")) // evicts "a"

	_, ok := c.Get("a@example.com")
	if ok {
		t.Fatal("expected eviction of oldest entry")
	}

	_, ok = c.Get("c@example.com")
	if !ok {
		t.Fatal("expected newest entry to remain")
	}
}

func TestMemoryCache_EmptyGroups(t *testing.T) {
	c, err := cache.NewMemoryCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("user@example.com", ug())

	got, ok := c.Get("user@example.com")
	if !ok {
		t.Fatal("expected cache hit for empty groups")
	}

	if len(got.Groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(got.Groups))
	}
}

// TestCache_ImplementsInterface verifies that MemoryCache satisfies the Cache interface.
func TestCache_ImplementsInterface(t *testing.T) {
	c, err := cache.NewMemoryCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Use as interface.
	var iface cache.Cache = c

	iface.Set("test@example.com", ug("g1"))

	got, ok := iface.Get("test@example.com")
	if !ok {
		t.Fatal("expected cache hit via interface")
	}

	if len(got.Groups) != 1 || got.Groups[0] != "g1" {
		t.Fatalf("unexpected groups: %v", got.Groups)
	}
}
