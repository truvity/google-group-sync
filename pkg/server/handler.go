package server

import (
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/truvity/google-group-sync/pkg/cache"
	"github.com/truvity/google-group-sync/pkg/resolver"
)

type (
	// groupsRequest is the JSON body for POST /groups.
	groupsRequest struct {
		Email string `json:"email"`
	}

	// groupsResponse is the JSON response for POST /groups.
	groupsResponse struct {
		Groups []string `json:"groups"`
	}
)

// NewGroupsHandler creates a fiber handler for POST /groups.
func NewGroupsHandler(logger *slog.Logger, res resolver.GroupResolver, c *cache.Cache) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		var req groupsRequest
		if err := ctx.Bind().JSON(&req); err != nil {
			return sendProblem(ctx, problemBadRequest("invalid request body: "+err.Error()))
		}

		email := strings.TrimSpace(req.Email)
		if email == "" {
			return sendProblem(ctx, problemBadRequest("email is required"))
		}

		// Check cache first.
		if groups, ok := c.Get(email); ok {
			logger.DebugContext(ctx.Context(), "cache hit",
				slog.String("email", email),
				slog.Int("groups", len(groups)),
			)

			return ctx.JSON(groupsResponse{Groups: groups})
		}

		// Resolve from Google.
		groups, err := res.ResolveGroups(ctx.Context(), email)
		if err != nil {
			logger.ErrorContext(ctx.Context(), "failed to resolve groups",
				slog.String("email", email),
				slog.Any("error", err),
			)

			// If Google returned a 4xx (e.g., 400 Invalid Input for non-email memberKey),
			// propagate as 400 — the caller sent a bad email.
			if isGoogleClientError(err) {
				return sendProblem(ctx, problemBadRequest("invalid email: "+err.Error()))
			}

			return sendProblem(ctx, problemGoogleAPIError(err.Error()))
		}

		// Ensure non-null JSON array.
		if groups == nil {
			groups = []string{}
		}

		// Store in cache.
		c.Set(email, groups)

		logger.InfoContext(ctx.Context(), "resolved groups",
			slog.String("email", email),
			slog.Int("groups", len(groups)),
		)

		return ctx.JSON(groupsResponse{Groups: groups})
	}
}
