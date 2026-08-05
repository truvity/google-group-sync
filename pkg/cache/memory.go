package cache

import (
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

// Compile-time interface check.
var _ Cache = (*MemoryCache)(nil)

// MemoryCache is an in-memory LRU cache with per-entry TTL expiration.
// It is the default Cache implementation, suitable for single-process deployments
// (K8s pods, Lambda instances with warm containers).
type MemoryCache struct {
	inner *lru.Cache[string, entry]
	ttl   time.Duration
}

type entry struct {
	ug        UserGroups
	expiresAt time.Time
}

// NewMemoryCache creates a new MemoryCache with the given max size and TTL.
func NewMemoryCache(maxSize int, ttl time.Duration) (*MemoryCache, error) {
	inner, err := lru.New[string, entry](maxSize)
	if err != nil {
		return nil, err
	}

	return &MemoryCache{inner: inner, ttl: ttl}, nil
}

// Get returns the cached resolution for the key, or (zero, false) if not
// found or expired.
func (c *MemoryCache) Get(key string) (UserGroups, bool) {
	e, ok := c.inner.Get(key)
	if !ok {
		return UserGroups{}, false
	}

	if time.Now().After(e.expiresAt) {
		c.inner.Remove(key)

		return UserGroups{}, false
	}

	return e.ug, true
}

// Set stores a resolution for the key with the configured TTL.
func (c *MemoryCache) Set(key string, ug UserGroups) {
	c.inner.Add(key, entry{
		ug:        ug,
		expiresAt: time.Now().Add(c.ttl),
	})
}

// Len returns the number of entries in the cache.
func (c *MemoryCache) Len() int {
	return c.inner.Len()
}
