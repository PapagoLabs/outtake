// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package respond

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/failure"
	"github.com/PapagoLabs/outtake/internal/metadata"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// flashProbePath is the route flashTestApp mounts to report the waiting
// failure.
const flashProbePath = "/_flash"

var (
	// errFailureTest is the failure the tests hand to Fail.
	errFailureTest = errors.New("disk full")

	// errTokenLeak is a failure whose text carries Plex tokens.
	errTokenLeak = errors.New(
		"get https://pms.local/library?X-Plex-Token=query-secret: header X-Plex-Token: header-secret",
	)
)

// flashTestApp builds an app with an in-memory session and a probe that
// reports the failure waiting for the next page.
//
// Parameters:
//   - t: The test the app belongs to.
//
// Returns:
//   - app: The app, ready for the routes under test.
func flashTestApp(t *testing.T) *fiber.App {
	t.Helper()

	middleware, store := session.NewWithStore(session.Config{})
	require.NotNil(t, store)

	app := newTestApp()
	app.Use(middleware)
	app.Get(flashProbePath, func(ctx fiber.Ctx) error {
		return ctx.JSON(PendingFlash(ctx))
	})

	return app
}

// flashAfter follows a response's session cookies to the probe.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: An app from flashTestApp.
//   - resp: The response whose session is read.
//
// Returns:
//   - failure: The waiting failure.
func flashAfter(t *testing.T, app *fiber.App, resp *http.Response) view.Failure {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, flashProbePath, nil)
	for _, cookie := range resp.Cookies() {
		req.AddCookie(cookie)
	}

	probe, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, probe)

	var failure view.Failure

	require.NoError(t, json.NewDecoder(probe.Body).Decode(&failure))

	return failure
}

// signIn marks a request's session as the owner's, so failures carry their
// details.
//
// Parameters:
//   - ctx: The request.
//   - token: The Plex token the session carries.
func signIn(ctx fiber.Ctx, token string) {
	sess := session.FromContext(ctx)

	identity.SetToken(sess, token)
	identity.SetUserID(sess, 42)
}

// failureFor runs Fail or FailWith inside a signed-in request and returns
// what it built.
//
// Parameters:
//   - t: The test the request belongs to.
//   - target: The request path and query.
//   - build: Builds the failure from the request.
//
// Returns:
//   - failure: What build returned.
func failureFor(t *testing.T, target string, build func(fiber.Ctx) view.Failure) view.Failure {
	t.Helper()

	var failure view.Failure

	app := flashTestApp(t)
	app.Post("/*", func(ctx fiber.Ctx) error {
		signIn(ctx, "owner-token")

		failure = build(ctx)

		return nil
	})

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, nil))
	require.NoError(t, err)

	defer closeBody(t, resp)

	return failure
}

// TestFailWithWritesTheDetailsBlock covers the details a page offers to copy:
// the version, the time, and a reference on the first line, the request on
// the second without its query, and the error chain last.
func TestFailWithWritesTheDetailsBlock(t *testing.T) {
	t.Parallel()

	failure := failureFor(t, "/api/clips?start=10", func(ctx fiber.Ctx) view.Failure {
		return FailWith(ctx, "Couldn't save the clip", fmt.Errorf("save clip: %w", errFailureTest))
	})

	assert.Equal(t, "Couldn't save the clip", failure.Message)

	lines := strings.Split(failure.Details, "\n")
	require.Len(t, lines, 3)
	assert.True(t, strings.HasPrefix(lines[0], "Outtake "+metadata.String()+" · "), lines[0])
	assert.Regexp(t, `· \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} UTC · ref [0-9a-f]{6}$`, lines[0])
	assert.Equal(
		t,
		"POST /api/clips",
		lines[1],
		"the query, which can carry form values, is left out",
	)
	assert.Equal(t, "save clip: disk full", lines[2])
}

// TestFailWithKeepsDetailsFromAVisitor covers a request no one signed in to:
// it gets the message alone, so an error chain never reaches someone who is
// not the owner.
func TestFailWithKeepsDetailsFromAVisitor(t *testing.T) {
	t.Parallel()

	var failure view.Failure

	app := flashTestApp(t)
	app.Post("/x", func(ctx fiber.Ctx) error {
		failure = FailWith(ctx, "Couldn't start the Plex login", errFailureTest)

		return nil
	})

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", nil))
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, view.NewNotice("Couldn't start the Plex login"), failure)
}

// TestFailWithAMessageAloneHasNoDetails covers a refusal the message explains
// in full: there is no error, so there are no details.
func TestFailWithAMessageAloneHasNoDetails(t *testing.T) {
	t.Parallel()

	failure := failureFor(t, "/x", func(ctx fiber.Ctx) view.Failure {
		return FailWith(ctx, "Paste a Plex token", nil)
	})

	assert.Equal(t, view.NewNotice("Paste a Plex token"), failure)
}

// TestFailKeepsDetailsOffAnInputRefusal covers Fail: a refusal of what the
// user entered shows its message alone, and any other failure carries
// details.
func TestFailKeepsDetailsOffAnInputRefusal(t *testing.T) {
	t.Parallel()

	refusal := failureFor(t, "/x", func(ctx fiber.Ctx) view.Failure {
		return Fail(ctx, fmt.Errorf("validate: %w", clip.ErrEmptyRange))
	})
	assert.Equal(t, view.NewNotice("The end must be after the start"), refusal)

	unexpected := failureFor(t, "/x", func(ctx fiber.Ctx) view.Failure {
		return Fail(ctx, errFailureTest)
	})
	assert.Equal(t, failure.MessageUnexpected, unexpected.Message)
	assert.Contains(t, unexpected.Details, "disk full")
}

// TestFailWithScrubsPlexTokens covers the rule that details never carry a
// Plex token: an X-Plex-Token in a URL or a header line, and the session's
// own token wherever it appears.
func TestFailWithScrubsPlexTokens(t *testing.T) {
	t.Parallel()

	const sessionToken = "session-secret-token"

	var failure view.Failure

	app := flashTestApp(t)
	app.Post("/x", func(ctx fiber.Ctx) error {
		signIn(ctx, sessionToken)

		failure = FailWith(ctx, "Couldn't reach the Plex server",
			fmt.Errorf("%w, body %s", errTokenLeak, sessionToken))

		return nil
	})

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", nil))
	require.NoError(t, err)

	defer closeBody(t, resp)

	for _, secret := range []string{"query-secret", "header-secret", sessionToken} {
		assert.NotContains(t, failure.Details, secret)
	}

	assert.Contains(t, failure.Details, "X-Plex-Token=[redacted]")
	assert.Contains(t, failure.Details, "body [redacted]")
}

// TestRenderHTMLShowsTheFlashOnce covers the session carrying a failure to the
// next page: a full page takes it into its banner slot once, and an htmx
// fragment, such as a background poll, leaves it for the page.
func TestRenderHTMLShowsTheFlashOnce(t *testing.T) {
	t.Parallel()

	app := flashTestApp(t)
	app.Post("/fail", func(ctx fiber.Ctx) error {
		SetFlash(ctx, view.Failure{Message: "Couldn't save the clip", Details: "ref 1"})

		return RedirectTo(ctx, "/page")
	})
	app.Get("/page", func(ctx fiber.Ctx) error {
		return RenderHTML(ctx, func(writer io.Writer) error {
			_, err := io.WriteString(writer, view.FailureFromContext(ctx.Context()).Message)

			return err //nolint:wrapcheck // The test writer's error is passed on.
		})
	})

	failed, err := app.Test(
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/fail", nil),
	)
	require.NoError(t, err)

	defer closeBody(t, failed)

	cookies := failed.Cookies()

	page := func(htmx bool) string {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/page", nil)
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}

		if htmx {
			req.Header.Set("Hx-Request", "true")
		}

		resp, testErr := app.Test(req)
		require.NoError(t, testErr)

		defer closeBody(t, resp)

		return bodyText(t, resp)
	}

	assert.Empty(t, page(true), "a fragment leaves the failure for the page")
	assert.Equal(t, "Couldn't save the clip", page(false))
	assert.Empty(t, page(false), "the failure shows once")
}

// TestWriteFailureSendsALinkBackToItsPage covers a link the browser followed,
// such as Download: the failure goes back to the page in the session rather
// than as a JSON body the browser would save.
func TestWriteFailureSendsALinkBackToItsPage(t *testing.T) {
	t.Parallel()

	app := flashTestApp(t)
	app.Get("/api/clips/c1/download", func(ctx fiber.Ctx) error {
		return WriteError(ctx, fiber.StatusConflict, api.NotReady, "This clip isn't ready yet")
	})

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/api/clips/c1/download",
		nil,
	)
	req.Header.Set(fiber.HeaderAccept, "text/html,application/xhtml+xml")
	req.Header.Set(fiber.HeaderReferer, "http://localhost:8080/clips")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/clips", resp.Header.Get(fiber.HeaderLocation))
	assert.Equal(t, "This clip isn't ready yet", flashAfter(t, app, resp).Message)
}

// TestWriteFailureGivesAnAPICallerTheDetails covers the JSON answer: the code
// stays the contract, the message is the plain one, and the details ride
// beside it.
func TestWriteFailureGivesAnAPICallerTheDetails(t *testing.T) {
	t.Parallel()

	app := flashTestApp(t)
	app.Post("/api/clips", func(ctx fiber.Ctx) error {
		return WriteFailure(ctx, fiber.StatusInternalServerError, api.PersistFailed,
			view.Failure{Message: "Couldn't save the clip", Details: "ref 1"})
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/clips", nil)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	var body api.ErrorResponse

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, api.ErrorResponse{
		Error:   api.PersistFailed,
		Message: "Couldn't save the clip",
		Details: "ref 1",
	}, body)
}
