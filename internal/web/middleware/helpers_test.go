// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"
)

// answer is everything a white-box test observes about one response.
type answer struct {
	// status is the response status code.
	status int
	// location is the Location response header.
	location string
	// body is the response body, read to end.
	body string
}

// issueRequest sends a request through the application under test.
//
// Parameters:
//   - t: The test that issues the request.
//   - app: The Fiber application serving the middleware chain.
//   - method: HTTP method of the request.
//   - target: Request path.
//
// Returns:
//   - got: The response the application produced.
func issueRequest(t *testing.T, app *fiber.App, method, target string) answer {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, target, http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return answer{
		status:   resp.StatusCode,
		location: resp.Header.Get(fiber.HeaderLocation),
		body:     string(body),
	}
}

// chainApp builds a Fiber application serving an ordered middleware chain.
//
// Parameters:
//   - t: The test that owns the application.
//   - last: The terminal handler the chain reaches.
//   - middle: The middleware to run, in order, before the terminal handler.
//
// Returns:
//   - app: An application that answers every request through the chain.
func chainApp(t *testing.T, last fiber.Handler, middle ...fiber.Handler) *fiber.App {
	t.Helper()

	app := fiber.New()

	for _, handler := range middle {
		app.Use(handler)
	}

	app.Use(func(ctx fiber.Ctx) error {
		return last(ctx)
	})

	return app
}

// okHandler answers with a bare success status.
//
// Returns:
//   - handler: A terminal handler reporting success.
func okHandler() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		return ctx.SendStatus(fiber.StatusOK)
	}
}

// teapotHandler answers with a route error instead of a response.
//
// Returns:
//   - handler: A terminal handler failing with fiber.ErrTeapot.
func teapotHandler() fiber.Handler {
	return func(fiber.Ctx) error {
		return fiber.ErrTeapot
	}
}

// writeHandler answers with a fixed string, so a test can tell which handler ran.
//
// Parameters:
//   - body: The response body to send.
//
// Returns:
//   - handler: A terminal handler answering with body.
func writeHandler(body string) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		return ctx.SendString(body)
	}
}
