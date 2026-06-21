package cache

import (
	"time"
)

// New creates a new MemoryCache with the given max size and TTL.
// This is a convenience alias for NewMemoryCache, preserving backward compatibility.
func New(maxSize int, ttl time.Duration) (*MemoryCache, error) {
	return NewMemoryCache(maxSize, ttl)
}
