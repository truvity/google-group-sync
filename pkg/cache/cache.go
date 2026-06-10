// Package cache provides an in-memory LRU cache with TTL for group resolution results.
package cache

import (
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

// Cache is an in-memory LRU cache with per-entry TTL expiration.
type Cache struct {
	inner *lru.Cache[string, entry]
	ttl   time.Duration
}

type entry struct {
	groups    []string
	expiresAt time.Time
}

// New creates a new Cache with the given max size and TTL.
func New(maxSize int, ttl time.Duration) (*Cache, error) {
	inner, err := lru.New[string, entry](maxSize)
	if err != nil {
		return nil, err
	}

	return &Cache{inner: inner, ttl: ttl}, nil
}

// Get returns cached groups for the email, or nil if not found or expired.
func (c *Cache) Get(email string) ([]string, bool) {
	e, ok := c.inner.Get(email)
	if !ok {
		return nil, false
	}

	if time.Now().After(e.expiresAt) {
		c.inner.Remove(email)

		return nil, false
	}

	return e.groups, true
}

// Set stores groups for the email with the configured TTL.
func (c *Cache) Set(email string, groups []string) {
	c.inner.Add(email, entry{
		groups:    groups,
		expiresAt: time.Now().Add(c.ttl),
	})
}

// Len returns the number of entries in the cache.
func (c *Cache) Len() int {
	return c.inner.Len()
}
