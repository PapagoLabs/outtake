// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package respondtest lets handler tests see the failure a handler left in
// the session for the next page, which a redirect carries there instead of
// the address.
package respondtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// ProbePath is the route Sessions mounts to report the waiting failure.
const ProbePath = "/_test/flash"

// Sessions gives app an in-memory session, so a failure a handler keeps for
// the next page survives the redirect, and mounts a probe that reports it
// without clearing it. Call it before mounting the routes under test.
//
// Parameters:
//   - t: The test the app belongs to.
//   - app: The app under test.
func Sessions(t *testing.T, app *fiber.App) {
	t.Helper()

	middleware, store := session.NewWithStore(session.Config{})
	require.NotNil(t, store)

	app.Use(middleware)
	app.Get(ProbePath, func(ctx fiber.Ctx) error {
		return ctx.JSON(respond.PendingFlash(ctx))
	})
}

// Flash follows a response's session cookies to the probe and returns the
// failure the next page shows.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: An app Sessions set up.
//   - resp: The response whose session is read.
//
// Returns:
//   - failure: The waiting failure, empty when there is none.
func Flash(t *testing.T, app *fiber.App, resp *http.Response) view.Failure {
	t.Helper()

	return FlashForCookies(t, app, resp.Cookies())
}

// FlashForCookies reads the failure waiting in the session the cookies name.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: An app Sessions set up.
//   - cookies: The browser's cookies after the request under test.
//
// Returns:
//   - failure: The waiting failure, empty when there is none.
func FlashForCookies(t *testing.T, app *fiber.App, cookies []*http.Cookie) view.Failure {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, ProbePath, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	var failure view.Failure

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&failure))

	return failure
}
