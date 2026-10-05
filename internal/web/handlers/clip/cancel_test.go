// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

type cancelAnswer struct {
	status   int
	location string
	header   http.Header
	body     string
}
type clipAnswer struct {
	status int
	body   string
}

// cancelTestHandler builds a handler whose queue holds one pending clip.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - status: Status the clip is queued under.
//   - mediaID: Media id the clip is cut from, empty for a clip with no source.
//
// Returns:
//   - handler: The handler under test.
//   - db: The database the queue persists status changes through.
func cancelTestHandler(
	t *testing.T,
	status clipdom.Status,
	mediaID string,
) (*Handler, *database.DB) {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	job := testClipJob("cancel-me", clipdom.TypeClip)

	job.Status = status
	job.MediaID = mediaID
	job.StartTime = 10 * time.Second
	job.Duration = 15 * time.Second
	job.CreatedAt = time.Now().UTC().Truncate(time.Second)
	job.UpdatedAt = job.CreatedAt
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)
	jobQueue.Restore(job)

	// The queue persists a status change through the callback the composition
	// root registers, so the stored row agrees with what the queue decided.
	jobQueue.SetStatusFunc(func(changed *clipdom.Job) {
		persistErr := db.SaveClip(context.WithoutCancel(t.Context()), changed)
		if persistErr != nil {
			t.Errorf("failed to persist clip status: %v", persistErr)
		}
	})

	return New(jobQueue, &blob.Storage{}, blob.Paths{}, db,
		&config.Config{MaxClipDur: 15 * time.Minute}, nil), db
}

// cancelAnswer is what one served cancel request produced.

// cancelClip serves one cancel request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - id: Clip id route parameter.
//   - contentType: Content type to post with, empty for none.
//   - htmx: Whether to mark the request as coming from HTMX.
//
// Returns:
//   - answer: The status, location, headers, and body the handler wrote.
func cancelClip(
	t *testing.T,
	handler *Handler,
	id, contentType string,
	htmx bool,
) cancelAnswer {
	t.Helper()

	app := fiber.New()
	app.Post("/api/clips/:id/cancel", handler.Cancel)

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/"+id+"/cancel", strings.NewReader(""),
	)
	if contentType != "" {
		req.Header.Set(fiber.HeaderContentType, contentType)
	}

	if htmx {
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

func TestCancelRejectsAClipNothingIsRegisteredUnder(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusPending, "42")

	answer := cancelClip(t, handler, "never-made", "", false)

	assert.Equal(t, fiber.StatusNotFound, answer.status)
	assert.Contains(t, answer.body, api.NotFound,
		"the code names what was missing")
}

func TestCancelRefusesAClipThatIsNotRunning(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusCompleted, "42")

	answer := cancelClip(t, handler, "cancel-me", "", false)

	assert.Equal(t, fiber.StatusConflict, answer.status)
	assert.Contains(t, answer.body, api.NotCancellable)
	assert.Contains(t, answer.body, "clip is not running")
}

func TestCancelTellsHTMXToRefreshThePage(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusPending, "42")

	answer := cancelClip(t, handler, "cancel-me", "", true)

	assert.Equal(t, fiber.StatusOK, answer.status)
	assert.Equal(t, "true", answer.header.Get(routes.HeaderHXRefresh),
		"the card swaps its own status, so htmx is told to reload instead")
	assert.NotContains(t, answer.body, "<",
		"a refresh answers with no markup, so nothing is swapped by accident")
}

func TestCancelRedirectsAFormPostBackToTheMediaItem(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusProcessing, "42")

	answer := cancelClip(t, handler, "cancel-me", fiber.MIMEApplicationForm, false)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, "/media/item/42", answer.location,
		"a form post goes back to the page the clip was cut from")
}

func TestCancelRedirectsAFormPostWithNoMediaToTheList(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusPending, "")

	answer := cancelClip(t, handler, "cancel-me", fiber.MIMEApplicationForm, false)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathClips, answer.location,
		"a clip with no media behind it has no item page to go back to")
}

func TestCancelAnswersAnAPICallerWithTheClip(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusPending, "42")

	answer := cancelClip(t, handler, "cancel-me", fiber.MIMEApplicationJSON, false)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, `"mediaId":"42"`,
		"the caller gets the clip it just stopped")
	assert.Contains(t, answer.body, `"progress":0`)
}

func TestCancelAnswersWithTheClipItLookedUp(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusPending, "42")

	answer := cancelClip(t, handler, "cancel-me", fiber.MIMEApplicationJSON, false)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, `"status":"pending"`,
		"catalog.Job hands back a copy taken before the cancel, so the body "+
			"still carries the status the handler found")
	assert.Equal(t, clipdom.StatusCancelled,
		handler.clipQueue.GetJob("cancel-me").Status,
		"the queue is what moved the clip on")
}

func TestCancelPersistsTheCanceledStatus(t *testing.T) {
	t.Parallel()

	handler, db := cancelTestHandler(t, clipdom.StatusProcessing, "42")

	require.Equal(t, fiber.StatusOK,
		cancelClip(t, handler, "cancel-me", fiber.MIMEApplicationJSON, false).status)

	stored, err := db.GetClip(t.Context(), "cancel-me")
	require.NoError(t, err)
	assert.Equal(t, clipdom.StatusCancelled, stored.Status,
		"the queue persists the change, so a reload does not resurrect the clip")
}

func TestListReturnsTheClipsUnderAnEnvelope(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusCompleted, "42")

	answer := listClips(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, `"clips":`,
		"the list is an envelope so a field can be added without breaking callers")
	assert.Contains(t, answer.body, `"id":"cancel-me"`)
	assert.Contains(t, answer.body, `"mediaId":"42"`)
}

func TestListPrefersTheQueueOverTheStoredRows(t *testing.T) {
	t.Parallel()

	handler, db := cancelTestHandler(t, clipdom.StatusCompleted, "42")

	stored := testClipJob("only-stored", clipdom.TypeGIF)

	stored.CreatedAt = time.Now().UTC().Truncate(time.Second)
	stored.UpdatedAt = stored.CreatedAt

	require.NoError(t, db.SaveClip(t.Context(), stored))

	answer := listClips(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, `"id":"cancel-me"`)
	assert.NotContains(t, answer.body, `"id":"only-stored"`,
		"a clip the queue owns is read from the queue, so a stale row cannot "+
			"report a status the render already moved past")
}

func TestListFallsBackToTheStoredRows(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusCompleted, "42")
	handler.clipQueue.Delete("cancel-me")

	answer := listClips(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, `"id":"cancel-me"`,
		"a settled clip is no longer in the queue, so the row is the answer")
}

func TestListIsAnEmptyEnvelopeWithNoClips(t *testing.T) {
	t.Parallel()

	handler, db := cancelTestHandler(t, clipdom.StatusCompleted, "42")

	handler.clipQueue.Delete("cancel-me")
	require.NoError(t, db.DeleteClip(t.Context(), "cancel-me"))

	answer := listClips(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.JSONEq(t, `{"clips":[]}`, answer.body,
		"an empty list is an empty array, not a missing key")
}

func TestGetStatusAnswersWithTheClip(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusProcessing, "42")

	answer := clipStatus(t, handler, "cancel-me")

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, `"id":"cancel-me"`)
	assert.Contains(t, answer.body, `"status":"processing"`)
}

func TestGetStatusRejectsAClipNothingIsRegisteredUnder(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusPending, "42")

	answer := clipStatus(t, handler, "never-made")

	assert.Equal(t, fiber.StatusNotFound, answer.status)
	assert.Contains(t, answer.body, api.NotFound)
}

// clipAnswer is what one served clip request produced.

// listClips serves one clip list request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - clipAnswer: The status and body the handler wrote.
func listClips(t *testing.T, handler *Handler) clipAnswer {
	t.Helper()

	app := fiber.New()
	app.Get("/api/clips", handler.List)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/clips", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return clipAnswer{status: resp.StatusCode, body: string(body)}
}

// clipStatus serves one clip status request against the handler.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - id: Clip id route parameter.
//
// Returns:
//   - clipAnswer: The status and body the handler wrote.
func clipStatus(t *testing.T, handler *Handler, id string) clipAnswer {
	t.Helper()

	app := fiber.New()
	app.Get("/api/clips/:id", handler.GetStatus)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/clips/"+id, nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return clipAnswer{status: resp.StatusCode, body: string(body)}
}

func TestSubmitFailureNamesAnAlreadyActiveClip(t *testing.T) {
	t.Parallel()

	failure := submitFailure(queue.ErrJobActive)

	assert.Equal(t, fiber.StatusConflict, failure.status)
	assert.Equal(t, api.JobActive, failure.code)
	assert.Contains(t, failure.message, queue.ErrJobActive.Error())
}

func TestSubmitFailureWrapsAnyOtherRejection(t *testing.T) {
	t.Parallel()

	failure := submitFailure(assert.AnError)

	assert.Equal(t, fiber.StatusInternalServerError, failure.status)
	assert.Equal(t, api.PersistFailed, failure.code)
	assert.Equal(t, assert.AnError.Error(), failure.message)
}

func TestSubmitFailureSeesAJobActiveWrappedInContext(t *testing.T) {
	t.Parallel()

	failure := submitFailure(wrapActive(t))

	assert.Equal(t, fiber.StatusConflict, failure.status,
		"the queue wraps ErrJobActive with the id it refused, and that still counts")
	assert.Equal(t, api.JobActive, failure.code)
}

func TestWriteJobSubmitErrorSwapsTheRejectionIntoTheFlash(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Post("/api/clips/submit", func(ctx fiber.Ctx) error {
		return writeJobSubmitError(ctx, queue.ErrJobActive)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/clips/submit", strings.NewReader(""),
	)
	req.Header.Set(routes.HeaderHXRequest, "true")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Contains(t, string(body), `hx-target="#flash"`,
		"the page has to show the rejection somewhere it can already reach")
	assert.Contains(t, string(body), queue.ErrJobActive.Error())
}

// wrapActive builds the queue's own ErrJobActive wrapper.
//
// Returns:
//   - err: The wrapped rejection the queue hands back.
func wrapActive(t *testing.T) error {
	t.Helper()

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	job := testClipJob("twice", clipdom.TypeClip)

	job.Status = clipdom.StatusPending

	require.NoError(t, jobQueue.Submit(job))

	err := jobQueue.Submit(job)
	require.Error(t, err)

	return fmt.Errorf("submit twice: %w", err)
}

func TestClipStatusPathIsEscaped(t *testing.T) {
	t.Parallel()

	handler, _ := cancelTestHandler(t, clipdom.StatusPending, "42")

	answer := clipStatus(t, handler, url.PathEscape("a/b"))

	assert.Equal(t, fiber.StatusNotFound, answer.status,
		"an id that is not a clip is not found rather than routed somewhere else")
}
