package cache_test

import (
	"testing"
	"time"

	"github.com/truvity/google-group-sync/pkg/cache"
)

func TestMemoryCache_SetAndGet(t *testing.T) {
	c, err := cache.NewMemoryCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	c.Set("user@example.com", []string{"group-a@example.com", "group-b@example.com"})

	groups, ok := c.Get("user@example.com")
	if !ok {
		t.Fatal("expected cache hit")
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	if groups[0] != "group-a@example.com" {
		t.Fatalf("expected group-a@example.com, got %s", groups[0])
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

	c.Set("user@example.com", []string{"group@example.com"})

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

	c.Set("a@example.com", []string{"g1"})
	c.Set("b@example.com", []string{"g2"})
	c.Set("c@example.com", []string{"g3"}) // evicts "a"

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

	c.Set("user@example.com", []string{})

	groups, ok := c.Get("user@example.com")
	if !ok {
		t.Fatal("expected cache hit for empty groups")
	}

	if len(groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(groups))
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

	iface.Set("test@example.com", []string{"g1"})

	groups, ok := iface.Get("test@example.com")
	if !ok {
		t.Fatal("expected cache hit via interface")
	}

	if len(groups) != 1 || groups[0] != "g1" {
		t.Fatalf("unexpected groups: %v", groups)
	}
}
