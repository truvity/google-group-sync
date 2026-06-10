package resolver

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"
)

// Compile-time interface check.
var _ GroupResolver = (*GoogleResolver)(nil)

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
	jwtConfig, err := google.JWTConfigFromJSON(r.saKeyJSON, admin.AdminDirectoryGroupReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse service account key: %w", err)
	}

	jwtConfig.Subject = r.adminEmail

	client := jwtConfig.Client(ctx)

	svc, err := admin.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create admin directory service: %w", err)
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
