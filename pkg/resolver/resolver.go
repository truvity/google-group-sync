// Package resolver provides the GroupResolver interface and implementations.
package resolver

import (
	"context"
)

// GroupResolver resolves Google Workspace group memberships for a user email.
type GroupResolver interface {
	// ResolveGroups returns the list of group email addresses that the given user belongs to.
	ResolveGroups(ctx context.Context, email string) ([]string, error)
}
