package resolver

import (
	"context"
	"log/slog"

	"golang.org/x/sync/singleflight"

	"github.com/truvity/google-group-sync/pkg/cache"
)

// Compile-time interface checks.
var (
	_ GroupResolver = (*CachedResolver)(nil)
	_ GroupLister   = (*CachedResolver)(nil)
)

// CachedResolver wraps a GroupLister with cache and singleflight deduplication.
// On cache miss, concurrent requests for the same email share a single in-flight call.
type CachedResolver struct {
	inner  GroupLister
	cache  cache.Cache
	flight singleflight.Group
	logger *slog.Logger
}

// NewCachedResolver creates a CachedResolver wrapping the given resolver and cache.
// The cache parameter accepts any implementation of cache.Cache (e.g., MemoryCache).
func NewCachedResolver(logger *slog.Logger, inner GroupLister, c cache.Cache) *CachedResolver {
	return &CachedResolver{
		inner:  inner,
		cache:  c,
		logger: logger,
	}
}

// flightResult carries resolved groups plus cache-hit info through singleflight.
type flightResult struct {
	groups []string
	cached bool
}

// ResolveGroups returns groups for the user, using cache and singleflight deduplication.
func (r *CachedResolver) ResolveGroups(ctx context.Context, email string) ([]string, error) {
	groups, _, err := r.ResolveGroupsCached(ctx, email)

	return groups, err
}

// ResolveGroupsCached is like ResolveGroups but also reports whether the result
// was served from cache.
func (r *CachedResolver) ResolveGroupsCached(ctx context.Context, email string) (groups []string, cached bool, err error) {
	// Check cache first.
	if groups, ok := r.cache.Get(email); ok {
		r.logger.DebugContext(ctx, "cache hit",
			slog.String("email", email),
			slog.Int("groups", len(groups)),
		)

		return groups, true, nil
	}

	// Deduplicate concurrent requests for the same email.
	v, err, shared := r.flight.Do("user:"+email, func() (interface{}, error) {
		// Double-check cache inside singleflight (another goroutine may have populated it).
		if groups, ok := r.cache.Get(email); ok {
			return flightResult{groups: groups, cached: true}, nil
		}

		groups, err := r.inner.ResolveGroups(ctx, email)
		if err != nil {
			return nil, err
		}

		// Ensure non-null slice.
		if groups == nil {
			groups = []string{}
		}

		r.cache.Set(email, groups)

		return flightResult{groups: groups}, nil
	})
	if err != nil {
		return nil, false, err
	}

	if shared {
		r.logger.DebugContext(ctx, "singleflight shared result",
			slog.String("email", email),
		)
	}

	res := v.(flightResult) //nolint:forcetypeassert // always flightResult from Do callback

	return res.groups, res.cached, nil
}

// ListGroups returns all groups with their members. This call is deduplicated via singleflight.
func (r *CachedResolver) ListGroups(ctx context.Context) ([]Group, error) {
	v, err, _ := r.flight.Do("list-all-groups", func() (interface{}, error) {
		return r.inner.ListGroups(ctx)
	})
	if err != nil {
		return nil, err
	}

	return v.([]Group), nil //nolint:forcetypeassert // always []Group from Do callback
}

// GetGroup returns a single group's members. This call is deduplicated via singleflight.
func (r *CachedResolver) GetGroup(ctx context.Context, groupEmail string) (*Group, error) {
	v, err, _ := r.flight.Do("group:"+groupEmail, func() (interface{}, error) {
		return r.inner.GetGroup(ctx, groupEmail)
	})
	if err != nil {
		return nil, err
	}

	return v.(*Group), nil //nolint:forcetypeassert // always *Group from Do callback
}
