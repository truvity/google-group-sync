// Package server provides the HTTP server for google-group-sync.
package server

import (
	"github.com/gofiber/fiber/v3"
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

func sendProblem(c fiber.Ctx, p *Problem) error {
	c.Set("Content-Type", "application/problem+json")

	return c.Status(p.Status).JSON(p)
}
