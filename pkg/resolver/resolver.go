// Package resolver provides the GroupResolver interface and implementations.
package resolver

import (
	"context"
	"errors"

	"github.com/truvity/google-group-sync/pkg/cache"
)

// UserGroups is one user's resolution: their group addresses, plus
// whether the directory reports the account SUSPENDED. The signal is
// read from the user's member entry (Members.Get status) — covered by
// the group-member scope this service already holds, no user-read
// delegation needed. Only the serving domain's own accounts ever carry
// it: an external member reports no status, which is the fail-safe
// reading — each directory vouches for suspension of ITS accounts only.
type UserGroups = cache.UserGroups

// ErrGroupNotFound is returned by GetGroup when the directory reports the
// group does not exist (an absent group), distinct from a read failure.
var ErrGroupNotFound = errors.New("group not found")

// GroupResolver resolves Google Workspace group memberships for a user email.
type GroupResolver interface {
	// ResolveGroups returns the list of group email addresses that the given user belongs to.
	ResolveGroups(ctx context.Context, email string) ([]string, error)

	// ResolveUser returns the groups plus the account's suspension
	// signal. Consumers deciding GRANTS want this one: a suspended
	// account resolving to zero groups is a revocation, while a plain
	// zero-group answer is ambiguous and fails safe.
	ResolveUser(ctx context.Context, email string) (UserGroups, error)
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

	// GetAccount returns one account's standing. found=false means the
	// directory returned not-found for the address; the caller decides what
	// that means from whether the address is in a served domain.
	GetAccount(ctx context.Context, email string) (Account, error)
}

// Account is one address's standing in this directory. Found=false is a
// clean not-found (deleted/absent); the in-domain-vs-not distinction is the
// caller's, from the served domains this resolver reports.
type Account struct {
	Email      string `json:"email"`
	Found      bool   `json:"found"`
	Live       bool   `json:"live"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}
