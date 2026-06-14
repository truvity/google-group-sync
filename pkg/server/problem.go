// Package server provides the HTTP server for google-group-sync.
package server

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/api/googleapi"
)

// Problem represents an RFC 9457 Problem Details response.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

const (
	problemBaseURL = "https://github.com/truvity/google-group-sync/problems"
)

func problemBadRequest(detail string) *Problem {
	return &Problem{
		Type:   problemBaseURL + "/bad-request",
		Title:  "Bad Request",
		Status: fiber.StatusBadRequest,
		Detail: detail,
	}
}

func problemGoogleAPIError(detail string) *Problem {
	return &Problem{
		Type:   problemBaseURL + "/google-api-error",
		Title:  "Google API Error",
		Status: fiber.StatusBadGateway,
		Detail: detail,
	}
}

func problemNotFound(detail string) *Problem {
	return &Problem{
		Type:   problemBaseURL + "/not-found",
		Title:  "Not Found",
		Status: fiber.StatusNotFound,
		Detail: detail,
	}
}

func sendProblem(c fiber.Ctx, p *Problem) error {
	c.Set("Content-Type", "application/problem+json")

	return c.Status(p.Status).JSON(p)
}

// isGoogleClientError returns true if the error wraps a Google API error with a 4xx status code.
func isGoogleClientError(err error) bool {
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code >= 400 && apiErr.Code < 500
	}

	return false
}
