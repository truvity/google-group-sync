package server

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/truvity/google-group-sync/pkg/resolver"
)

type (
	// getGroupResponse is the JSON response for GET /groups/{email}.
	getGroupResponse struct {
		Email   string   `json:"email"`
		Members []string `json:"members"`
	}

	// userGroupsResponse is the JSON response for GET /users/{email}/groups.
	userGroupsResponse struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}
)

// NewListGroupsHandler creates a fiber handler for GET /groups (no query param).
// Lists all groups with their members.
func NewListGroupsHandler(logger *slog.Logger, res resolver.GroupLister) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Backward compat: if ?email=X is present, redirect to /users/{email}/groups.
		email := strings.TrimSpace(c.Query("email"))
		if email != "" {
			return c.Redirect().Status(fiber.StatusFound).To("/users/" + email + "/groups")
		}

		groups, err := res.ListGroups(c.Context())
		if err != nil {
			logger.ErrorContext(c.Context(), "failed to list groups",
				slog.Any("error", err),
			)

			if isGoogleClientError(err) {
				return sendProblem(c, problemBadRequest("invalid request: "+err.Error()))
			}

			return sendProblem(c, problemGoogleAPIError(err.Error()))
		}

		if groups == nil {
			groups = []resolver.Group{}
		}

		return c.JSON(groups)
	}
}

// NewGetGroupHandler creates a fiber handler for GET /groups/{email}.
// Returns a single group's members.
func NewGetGroupHandler(logger *slog.Logger, res resolver.GroupLister) fiber.Handler {
	return func(c fiber.Ctx) error {
		groupEmail := strings.TrimSpace(c.Params("email"))
		if groupEmail == "" {
			return sendProblem(c, problemBadRequest("group email is required"))
		}

		group, err := res.GetGroup(c.Context(), groupEmail)
		if err != nil {
			logger.ErrorContext(c.Context(), "failed to get group",
				slog.String("group", groupEmail),
				slog.Any("error", err),
			)

			if isGoogleClientError(err) {
				return sendProblem(c, problemNotFound("group not found: "+groupEmail))
			}

			return sendProblem(c, problemGoogleAPIError(err.Error()))
		}

		return c.JSON(getGroupResponse{
			Email:   group.Email,
			Members: group.Members,
		})
	}
}

// cacheAwareResolver reports whether a lookup was served from cache
// (implemented by resolver.CachedResolver).
type cacheAwareResolver interface {
	ResolveGroupsCached(ctx context.Context, email string) ([]string, bool, error)
}

// NewUserGroupsHandler creates a fiber handler for GET /users/{email}/groups.
// Returns all groups a user belongs to.
func NewUserGroupsHandler(logger *slog.Logger, res resolver.GroupLister) fiber.Handler {
	return func(c fiber.Ctx) error {
		email := strings.TrimSpace(c.Params("email"))
		if email == "" {
			return sendProblem(c, problemBadRequest("user email is required"))
		}

		var (
			groups []string
			cached bool
			err    error
		)

		if car, ok := res.(cacheAwareResolver); ok {
			groups, cached, err = car.ResolveGroupsCached(c.Context(), email)
		} else {
			groups, err = res.ResolveGroups(c.Context(), email)
		}

		if err != nil {
			logger.ErrorContext(c.Context(), "failed to resolve user groups",
				slog.String("email", email),
				slog.Any("error", err),
			)

			if isGoogleClientError(err) {
				return sendProblem(c, problemBadRequest("invalid email: "+err.Error()))
			}

			return sendProblem(c, problemGoogleAPIError(err.Error()))
		}

		if groups == nil {
			groups = []string{}
		}

		if len(groups) == 0 {
			logger.WarnContext(c.Context(), "user groups lookup returned no groups",
				slog.String("email", email),
				slog.Int("groups", 0),
				slog.Bool("cached", cached),
			)
		} else {
			logger.InfoContext(c.Context(), "resolved user groups",
				slog.String("email", email),
				slog.Int("groups", len(groups)),
				slog.Bool("cached", cached),
			)
		}

		// Group names only at DEBUG (too noisy for INFO).
		logger.DebugContext(c.Context(), "user groups detail",
			slog.String("email", email),
			slog.Any("groups", groups),
		)

		return c.JSON(userGroupsResponse{
			Email:  email,
			Groups: groups,
		})
	}
}
