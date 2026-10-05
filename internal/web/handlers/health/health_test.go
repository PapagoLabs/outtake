// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package health

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"
)

// healthAnswer is what one served health request produced.
type healthAnswer struct {
	status int
	header http.Header
	body   string
}

// closeBody closes a response body and fails the test when it cannot.
//
// Parameters:
//   - t: The test the response belongs to.
//   - resp: The response to close.
func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	require.NoError(t, resp.Body.Close())
}

// getHealth serves one health request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - answer: The status and body the handler wrote.
func getHealth(t *testing.T, handler *Handler) healthAnswer {
	t.Helper()

	app := fiber.New()
	app.Get("/api/health", handler.Health)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/health", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	return healthAnswer{
		status: resp.StatusCode,
		header: resp.Header,
		body:   readBody(t, resp),
	}
}

// readBody reads a response body in full.
//
// Parameters:
//   - t: The test the response belongs to.
//   - resp: The response to drain.
//
// Returns:
//   - body: The bytes the response carried.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body)
}

func TestNewBuildsAHandlerWithNoCollaborators(t *testing.T) {
	t.Parallel()

	assert.Equal(t, &Handler{}, New(),
		"the health check answers on its own, so it carries no collaborators")
}

func TestHealthReportsTheServiceIsUp(t *testing.T) {
	t.Parallel()

	answer := getHealth(t, New())

	assert.Equal(t, fiber.StatusOK, answer.status)
	assert.JSONEq(t, `{"status":"ok","service":"outtake"}`, answer.body,
		"a probe needs both fields to tell outtake apart from any other service")
}

func TestHealthNeedsNoSessionOrPlexServer(t *testing.T) {
	t.Parallel()

	answer := getHealth(t, New())

	assert.Empty(t, answer.header.Get(fiber.HeaderSetCookie),
		"a health check must not start a session for the caller")
	assert.Contains(t, answer.header.Get(fiber.HeaderContentType),
		fiber.MIMEApplicationJSON,
		"a probe reads the body as JSON, so that is what the response has to be")
}

func TestHealthAnswersEveryMethodRoutedToIt(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			app.Add([]string{method}, "/api/health", New().Health)

			resp, err := app.Test(httptest.NewRequestWithContext(
				t.Context(), method, "/api/health", nil,
			))
			require.NoError(t, err)

			defer closeBody(t, resp)

			assert.Equal(t, fiber.StatusOK, resp.StatusCode,
				"a monitor must not have to guess which verb the probe uses")
		})
	}
}
