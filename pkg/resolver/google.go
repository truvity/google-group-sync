package resolver

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"

	"github.com/truvity/google-group-sync/pkg/keysource"
)

// Compile-time interface checks.
var (
	_ GroupResolver = (*GoogleResolver)(nil)
	_ GroupLister   = (*GoogleResolver)(nil)
)

// GoogleResolver implements GroupResolver using the Google Admin SDK Directory API.
// It uses domain-wide delegation with a service account impersonating an admin user.
//
// The key comes from a keysource.Source and is fetched on every service
// construction: a rotated key (kubelet-refreshed Secret mount) is picked
// up by the next call with no restart.
type GoogleResolver struct {
	logger     *slog.Logger
	keys       keysource.Source
	adminEmail string
}

// NewGoogleResolver creates a new GoogleResolver.
// keys yields the service-account key content (static or file-backed).
// adminEmail is the admin user to impersonate for domain-wide delegation.
func NewGoogleResolver(logger *slog.Logger, keys keysource.Source, adminEmail string) *GoogleResolver {
	return &GoogleResolver{
		logger:     logger,
		keys:       keys,
		adminEmail: adminEmail,
	}
}

// ResolveGroups lists all groups the user belongs to using the Google Admin SDK.
func (r *GoogleResolver) ResolveGroups(ctx context.Context, email string) ([]string, error) {
	ug, err := r.ResolveUser(ctx, email)
	if err != nil {
		return nil, err
	}

	return ug.Groups, nil
}

// ResolveUser lists the user's groups and reads the account's suspension
// signal from their member entry in one of them.
func (r *GoogleResolver) ResolveUser(ctx context.Context, email string) (UserGroups, error) {
	svc, err := r.newService(ctx)
	if err != nil {
		return UserGroups{}, err
	}

	var groups []string

	call := svc.Groups.List().UserKey(email)

	err = call.Pages(ctx, func(resp *admin.Groups) error {
		for i := range resp.Groups {
			groups = append(groups, resp.Groups[i].Email)
		}

		return nil
	})
	if err != nil {
		return UserGroups{}, fmt.Errorf("list groups for user %q: %w", email, err)
	}

	ug := UserGroups{Groups: groups, Suspended: r.memberSuspended(ctx, svc, email, groups)}

	r.logger.DebugContext(ctx, "resolved groups",
		slog.String("email", email),
		slog.Int("count", len(ug.Groups)),
		slog.Bool("suspended", ug.Suspended),
	)

	return ug, nil
}

// statusProbeLimit bounds how many of the user's groups are asked for
// their member entry before giving up on the status probe. The first
// direct membership answers in practice; the cap keeps a deeply nested
// pathological case from turning one resolve into a request storm.
const statusProbeLimit = 3

// memberSuspended reads the account's suspension signal from the user's
// member entry (Members.Get carries a status field, covered by the
// group-member scope — no user-read delegation required). Only a
// positive SUSPENDED counts: an external member, a nested membership the
// probe cannot see, or a probe error all read as not-suspended, because
// this signal REVOKES access downstream and must never fire on absence
// of evidence.
func (r *GoogleResolver) memberSuspended(ctx context.Context, svc *admin.Service, email string, groups []string) bool {
	for i, group := range groups {
		if i == statusProbeLimit {
			break
		}

		member, err := svc.Members.Get(group, email).Context(ctx).Do()
		if err != nil {
			// Typically a nested (indirect) membership: the user is in
			// the group via another group, so no direct member entry
			// exists. Try the next group.
			continue
		}

		return member.Status == "SUSPENDED"
	}

	return false
}

// ListGroups returns all groups in the domain with their members.
func (r *GoogleResolver) ListGroups(ctx context.Context) ([]Group, error) {
	svc, err := r.newService(ctx)
	if err != nil {
		return nil, err
	}

	var groups []Group

	// List all groups in the domain (customer = "my_customer" means the caller's domain).
	err = svc.Groups.List().Customer("my_customer").Pages(ctx, func(resp *admin.Groups) error {
		for i := range resp.Groups {
			g := resp.Groups[i]

			members, mErr := r.listGroupMembers(ctx, svc, g.Email)
			if mErr != nil {
				return mErr
			}

			groups = append(groups, Group{
				Email:   g.Email,
				Members: members,
			})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list all groups: %w", err)
	}

	r.logger.DebugContext(ctx, "listed all groups",
		slog.Int("count", len(groups)),
	)

	return groups, nil
}

// GetGroup returns a single group's members.
func (r *GoogleResolver) GetGroup(ctx context.Context, groupEmail string) (*Group, error) {
	svc, err := r.newService(ctx)
	if err != nil {
		return nil, err
	}

	// Verify the group exists.
	g, err := svc.Groups.Get(groupEmail).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get group %q: %w", groupEmail, err)
	}

	members, err := r.listGroupMembers(ctx, svc, g.Email)
	if err != nil {
		return nil, err
	}

	return &Group{
		Email:   g.Email,
		Members: members,
	}, nil
}

func (r *GoogleResolver) newService(ctx context.Context) (*admin.Service, error) {
	saKeyJSON, err := r.keys.Bytes()
	if err != nil {
		return nil, fmt.Errorf("load service account key: %w", err)
	}

	jwtConfig, err := google.JWTConfigFromJSON(saKeyJSON, admin.AdminDirectoryGroupReadonlyScope, admin.AdminDirectoryGroupMemberReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse service account key: %w", err)
	}

	jwtConfig.Subject = r.adminEmail

	client := jwtConfig.Client(ctx)

	svc, err := admin.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create admin directory service: %w", err)
	}

	return svc, nil
}

func (r *GoogleResolver) listGroupMembers(ctx context.Context, svc *admin.Service, groupEmail string) ([]string, error) {
	var members []string

	err := svc.Members.List(groupEmail).Pages(ctx, func(resp *admin.Members) error {
		for i := range resp.Members {
			members = append(members, resp.Members[i].Email)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list members for group %q: %w", groupEmail, err)
	}

	if members == nil {
		members = []string{}
	}

	return members, nil
}
