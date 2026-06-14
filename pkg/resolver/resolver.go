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

// Group represents a Google Workspace group with its members.
type Group struct {
	Email   string   `json:"email"`
	Members []string `json:"members"`
}

// GroupLister extends GroupResolver with the ability to list all groups and their members.
type GroupLister interface {
	GroupResolver

	// ListGroups returns all groups with their members.
	ListGroups(ctx context.Context) ([]Group, error)

	// GetGroup returns a single group's members by the group's email address.
	GetGroup(ctx context.Context, groupEmail string) (*Group, error)
}
