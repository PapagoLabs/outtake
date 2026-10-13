// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/middleware/middlewaretest"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// shareRequest is one request to a share route.
type shareRequest struct {
	// caller records who makes the request.
	caller fiber.Handler
	// action is "share" or "unshare".
	action string
	// contentType is the request's content type, empty for none.
	contentType string
	// htmx marks the request as coming from htmx.
	htmx bool
}

// shareClip serves one share or unshare request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - request: The request to make.
//
// Returns:
//   - answer: The status, location, headers, and body the handler wrote.
func shareClip(t *testing.T, handler *Handler, request shareRequest) cancelAnswer {
	t.Helper()

	app := fiber.New()
	app.Use(request.caller)
	app.Post("/api/clips/:id/share", handler.Share)
	app.Post("/api/clips/:id/unshare", handler.Unshare)

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/clips/cancel-me/"+request.action,
		strings.NewReader(""),
	)
	if request.contentType != "" {
		req.Header.Set(fiber.HeaderContentType, request.contentType)
	}

	if request.htmx {
		req.Header.Set(routes.HeaderHXRequest, "true")
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return cancelAnswer{
		status:   resp.StatusCode,
		location: resp.Header.Get(fiber.HeaderLocation),
		header:   resp.Header,
		body:     string(body),
	}
}

// storedShared reads whether the stored clip is shared.
//
// Parameters:
//   - t: The test that reads it.
//   - db: The database.
//
// Returns:
//   - shared: The stored flag.
func storedShared(t *testing.T, db *database.DB) bool {
	t.Helper()

	stored, err := db.GetClip(t.Context(), "cancel-me")
	require.NoError(t, err)

	return stored.Shared
}

// TestShareAndUnshareAnswerAnAPICaller covers sharing a clip and making it
// private again through the API.
//
// Each answers with the clip, and the change is saved.
func TestShareAndUnshareAnswerAnAPICaller(t *testing.T) {
	t.Parallel()

	handler, db := cancelTestHandler(t, clipdom.StatusCompleted, "42")

	shared := shareClip(t, handler, shareRequest{
		caller: middlewaretest.AsOwner(), action: "share", contentType: fiber.MIMEApplicationJSON,
	})
	require.Equal(t, fiber.StatusOK, shared.status)
	assert.Contains(t, shared.body, `"shared":true`)
	assert.True(t, storedShared(t, db))

	unshared := shareClip(t, handler, shareRequest{
		caller: middlewaretest.AsOwner(), action: "unshare", contentType: fiber.MIMEApplicationJSON,
	})
	require.Equal(t, fiber.StatusOK, unshared.status)
	assert.Contains(t, unshared.body, `"shared":false`)
	assert.False(t, storedShared(t, db))
}

// TestShareSwapsInTheClipCard covers the card's share button, whose htmx
// request gets the card back, now offering Stop Sharing.
func TestShareSwapsInTheClipCard(t *testing.T) {
	t.Parallel()

	handler, db := cancelTestHandler(t, clipdom.StatusCompleted, "42")

	// The card is filled in from its source, as on the page.
	handler.sources = stubSources(t, time.Minute)

	_, err := db.Conn().ExecContext(
		t.Context(),
		"INSERT INTO users (plex_user_id, username, role) VALUES (42, 'owner', 'owner'), "+
			"(7, 'friend', 'member')",
	)
	require.NoError(t, err)

	answer := shareClip(t, handler, shareRequest{
		caller:      middlewaretest.AsOwner(),
		action:      "share",
		contentType: fiber.MIMEApplicationForm,
		htmx:        true,
	})

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, `id="clip-cancel-me"`)
	assert.Contains(t, answer.body, `hx-post="/api/clips/cancel-me/unshare"`)
	assert.Contains(t, answer.body, "Stop Sharing")
}

// TestShareRedirectsAFormPostBack covers a form post without htmx, which is
// sent back to the page it came from.
func TestShareRedirectsAFormPostBack(t *testing.T) {
	t.Parallel()

	handler, db := cancelTestHandler(t, clipdom.StatusCompleted, "42")

	answer := shareClip(t, handler, shareRequest{
		caller: middlewaretest.AsOwner(), action: "share", contentType: fiber.MIMEApplicationForm,
	})

	assert.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.True(t, storedShared(t, db))
}

// TestShareRefusesAClipTheCallerMayNotChange covers a member asking to share
// a clip they did not make.
//
// The clip reads as missing and stays private.
func TestShareRefusesAClipTheCallerMayNotChange(t *testing.T) {
	t.Parallel()

	handler, db := cancelTestHandler(t, clipdom.StatusCompleted, "42")

	answer := shareClip(t, handler, shareRequest{
		caller: middlewaretest.AsMember(), action: "share", contentType: fiber.MIMEApplicationJSON,
	})

	assert.Equal(t, fiber.StatusNotFound, answer.status)
	assert.False(t, storedShared(t, db))
}
