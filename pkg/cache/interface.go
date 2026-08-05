// Package cache provides caching primitives for group resolution results.
package cache

// Cache defines the contract for a group resolution cache.
// Implementations must be safe for concurrent use.
//
// Get returns the cached groups for the given key and whether the entry was found
// (and not expired). A false return means the caller should resolve fresh data.
//
// Set stores groups for the given key. The implementation determines TTL and eviction.
//
// Future implementations:
//   - DynamoDBCache: shared cache for Lambda-at-scale deployments where multiple
//     concurrent Lambda instances benefit from a warm shared cache. Uses a DynamoDB
//     table with TTL attribute for automatic expiration. Not yet implemented —
//     MemoryCache is sufficient for the current traffic pattern.
type Cache interface {
	// Get returns the cached resolution for the key, or (zero, false) if
	// not found/expired.
	Get(key string) (UserGroups, bool)

	// Set stores a resolution for the key with implementation-defined TTL
	// semantics.
	Set(key string, ug UserGroups)
}

// UserGroups is one user's cached resolution: their group addresses,
// plus whether the directory reports the account suspended. Defined here
// rather than in resolver so the cache does not import its own consumer;
// resolver aliases it as the public name.
type UserGroups struct {
	Groups    []string `json:"groups"`
	Suspended bool     `json:"suspended"`
}
