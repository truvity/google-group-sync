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

// flightResult carries one resolution plus cache-hit info through singleflight.
type flightResult struct {
	ug     UserGroups
	cached bool
}

// ResolveGroups returns groups for the user, using cache and singleflight deduplication.
func (r *CachedResolver) ResolveGroups(ctx context.Context, email string) ([]string, error) {
	ug, _, err := r.ResolveUserCached(ctx, email)

	return ug.Groups, err
}

// ResolveUser returns the user's groups plus the suspension signal,
// using cache and singleflight deduplication.
func (r *CachedResolver) ResolveUser(ctx context.Context, email string) (UserGroups, error) {
	ug, _, err := r.ResolveUserCached(ctx, email)

	return ug, err
}

// ResolveUserCached is like ResolveUser but also reports whether the
// result was served from cache.
func (r *CachedResolver) ResolveUserCached(ctx context.Context, email string) (ug UserGroups, cached bool, err error) {
	// Check cache first.
	if ug, ok := r.cache.Get(email); ok {
		r.logger.DebugContext(ctx, "cache hit",
			slog.String("email", email),
			slog.Int("groups", len(ug.Groups)),
		)

		return ug, true, nil
	}

	// Deduplicate concurrent requests for the same email.
	v, err, shared := r.flight.Do("user:"+email, func() (interface{}, error) {
		// Double-check cache inside singleflight (another goroutine may have populated it).
		if ug, ok := r.cache.Get(email); ok {
			return flightResult{ug: ug, cached: true}, nil
		}

		ug, err := r.inner.ResolveUser(ctx, email)
		if err != nil {
			return nil, err
		}

		// Ensure non-null slice.
		if ug.Groups == nil {
			ug.Groups = []string{}
		}

		r.cache.Set(email, ug)

		return flightResult{ug: ug}, nil
	})
	if err != nil {
		return UserGroups{}, false, err
	}

	if shared {
		r.logger.DebugContext(ctx, "singleflight shared result",
			slog.String("email", email),
		)
	}

	res := v.(flightResult) //nolint:forcetypeassert // always flightResult from Do callback

	return res.ug, res.cached, nil
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

// GetAccount passes through to the inner resolver. Account standing is not
// cached: it is read on demand at reconcile time and is cheap relative to
// the group listings the cache exists for.
func (r *CachedResolver) GetAccount(ctx context.Context, email string) (Account, error) {
	return r.inner.GetAccount(ctx, email)
}
