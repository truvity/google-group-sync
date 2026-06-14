package resolver

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"
)

// Compile-time interface checks.
var (
	_ GroupResolver = (*GoogleResolver)(nil)
	_ GroupLister   = (*GoogleResolver)(nil)
)

// GoogleResolver implements GroupResolver using the Google Admin SDK Directory API.
// It uses domain-wide delegation with a service account impersonating an admin user.
type GoogleResolver struct {
	logger     *slog.Logger
	saKeyJSON  []byte
	adminEmail string
}

// NewGoogleResolver creates a new GoogleResolver.
// saKeyJSON is the service account key file content.
// adminEmail is the admin user to impersonate for domain-wide delegation.
func NewGoogleResolver(logger *slog.Logger, saKeyJSON []byte, adminEmail string) *GoogleResolver {
	return &GoogleResolver{
		logger:     logger,
		saKeyJSON:  saKeyJSON,
		adminEmail: adminEmail,
	}
}

// ResolveGroups lists all groups the user belongs to using the Google Admin SDK.
func (r *GoogleResolver) ResolveGroups(ctx context.Context, email string) ([]string, error) {
	svc, err := r.newService(ctx)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("list groups for user %q: %w", email, err)
	}

	r.logger.DebugContext(ctx, "resolved groups",
		slog.String("email", email),
		slog.Int("count", len(groups)),
	)

	return groups, nil
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
	jwtConfig, err := google.JWTConfigFromJSON(r.saKeyJSON, admin.AdminDirectoryGroupReadonlyScope, admin.AdminDirectoryGroupMemberReadonlyScope)
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
