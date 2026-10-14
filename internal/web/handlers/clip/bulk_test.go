// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/quota"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/middleware/middlewaretest"
	"github.com/PapagoLabs/outtake/internal/web/respond/respondtest"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// bulkCall is one request to a bulk route.
type bulkCall struct {
	// caller records who makes the request.
	caller fiber.Handler
	// path is the route.
	path string
	// body is the request body.
	body string
	// contentType is the body's type.
	contentType string
	// htmx marks the request as coming from htmx.
	htmx bool
}

// bulkAnswer is what a bulk route answered.
type bulkAnswer struct {
	// body is the raw response.
	body string
	// report is the JSON report, zero for an htmx answer.
	report api.BulkResponse
	// status is the response status.
	status int
}

// bulkTimeout bounds one bulk request. A batch of maxBulkClips clips checks
// and probes each one, which takes longer than Fiber's default second on a
// loaded CI runner under the race detector.
const bulkTimeout = 30 * time.Second

// postBulk serves one request to a bulk route.
//
// The caller's session holds a server token, so a member's Plex check
// reaches the stand-in server.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - call: The request.
//
// Returns:
//   - answer: The status, body, and JSON report.
func postBulk(t *testing.T, handler *Handler, call bulkCall) bulkAnswer {
	t.Helper()

	app := fiber.New()
	respondtest.Sessions(t, app)
	app.Use(call.caller)
	app.Use(func(ctx fiber.Ctx) error {
		identity.SetServerToken(session.FromContext(ctx), "member-token", time.Now())

		return ctx.Next()
	})
	app.Post("/api/clips/bulk", handler.Bulk)
	app.Post("/api/clips/retry", handler.RetryFailed)

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, call.path, strings.NewReader(call.body),
	)
	req.Header.Set(fiber.HeaderContentType, call.contentType)

	if call.htmx {
		req.Header.Set(routes.HeaderHXRequest, "true")
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: bulkTimeout, FailOnTimeout: true})
	require.NoError(t, err)

	defer closeBody(t, resp)

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	answer := bulkAnswer{body: string(raw), report: api.BulkResponse{}, status: resp.StatusCode}

	if strings.HasPrefix(resp.Header.Get(fiber.HeaderContentType), fiber.MIMEApplicationJSON) {
		_ = json.Unmarshal(raw, &answer.report)
	}

	return answer
}

// bulkJSON posts a JSON bulk request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - caller: Records who makes the request.
//   - action: The action.
//   - ids: The clips.
//
// Returns:
//   - answer: What the route answered.
func bulkJSON(
	t *testing.T,
	handler *Handler,
	caller fiber.Handler,
	action string,
	ids ...string,
) bulkAnswer {
	t.Helper()

	// A request of strings always encodes.
	body, _ := json.Marshal(api.BulkRequest{Action: action, Clips: ids})

	return postBulk(t, handler, bulkCall{
		caller:      caller,
		path:        "/api/clips/bulk",
		body:        string(body),
		contentType: fiber.MIMEApplicationJSON,
		htmx:        false,
	})
}

// storeOwned stores a clip of the handler's source that an account owns.
//
// Parameters:
//   - t: The test the clip belongs to.
//   - handler: The handler whose database holds it.
//   - source: The clip's source file.
//   - id: The clip.
//   - owner: The owning account.
//   - status: The clip's status.
func storeOwned(
	t *testing.T,
	handler *Handler,
	source, id string,
	owner int,
	status clipdom.Status,
) {
	t.Helper()

	job := testClipJob(id, clipdom.TypeClip)

	job.InputPath = source
	job.OwnerID = owner
	job.Status = status
	job.StartTime = time.Second
	job.Duration = 5 * time.Second
	require.NoError(t, handler.db.SaveClip(t.Context(), job))
}

// TestBulkDeletesEverySelectedClip covers Delete on several clips, named
// with a repeat and an empty id.
//
// Each clip is deleted once, and the report counts them.
func TestBulkDeletesEverySelectedClip(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)
	storeOwned(t, handler, source, "a", middlewaretest.OwnerID, clipdom.StatusCompleted)
	storeOwned(t, handler, source, "b", middlewaretest.OwnerID, clipdom.StatusFailed)

	answer := bulkJSON(t, handler, middlewaretest.AsOwner(), "delete", "a", "b", "a", "")

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, 2, answer.report.Done)
	assert.Equal(t, "Deleted 2 clips", answer.report.Message)

	stored, err := handler.db.ListClips(t.Context())
	require.NoError(t, err)
	assert.Empty(t, stored)
}

// TestBulkSkipsAClipTheCallerMayNotChange covers a member who names the
// owner's clip beside their own.
//
// Only the member's clip is deleted, and the owner's is skipped.
func TestBulkSkipsAClipTheCallerMayNotChange(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)
	storeOwned(t, handler, source, "mine", middlewaretest.MemberID, clipdom.StatusCompleted)
	storeOwned(t, handler, source, "theirs", middlewaretest.OwnerID, clipdom.StatusCompleted)

	answer := bulkJSON(t, handler, middlewaretest.AsMember(), "delete", "mine", "theirs", "absent")

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, 1, answer.report.Done)
	assert.Equal(t, 2, answer.report.Skipped)
	assert.Equal(t, "Deleted 1 clip. 2 clips were skipped.", answer.report.Message)

	_, err := handler.db.GetClip(t.Context(), "theirs")
	require.NoError(t, err, "the owner's clip is kept")
}

// TestBulkCancelsOnlyClipsThatRender covers Cancel on a waiting clip and a
// finished one.
//
// The waiting clip is canceled, and the finished one is skipped.
func TestBulkCancelsOnlyClipsThatRender(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)
	storeOwned(t, handler, source, "done", middlewaretest.OwnerID, clipdom.StatusCompleted)

	waiting := testClipJob("waiting", clipdom.TypeClip)

	waiting.Status = clipdom.StatusPending
	handler.clipQueue.Restore(waiting)

	answer := bulkJSON(t, handler, middlewaretest.AsOwner(), "cancel", "waiting", "done")

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, 1, answer.report.Done)
	assert.Equal(t, 1, answer.report.Skipped)
	assert.Equal(t, clipdom.StatusCancelled, handler.clipQueue.GetJob("waiting").Status)
}

// TestBulkSharesAndUnsharesClips covers Share and Stop Sharing on several
// clips.
func TestBulkSharesAndUnsharesClips(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)
	storeOwned(t, handler, source, "a", middlewaretest.OwnerID, clipdom.StatusCompleted)
	storeOwned(t, handler, source, "b", middlewaretest.OwnerID, clipdom.StatusCompleted)

	shared := bulkJSON(t, handler, middlewaretest.AsOwner(), "share", "a", "b")
	require.Equal(t, fiber.StatusOK, shared.status, shared.body)
	assert.Equal(t, "Shared 2 clips", shared.report.Message)
	assert.True(t, handler.clipQueue.GetJob("a").Shared)
	assert.True(t, handler.clipQueue.GetJob("b").Shared)

	unshared := bulkJSON(t, handler, middlewaretest.AsOwner(), "unshare", "a")
	require.Equal(t, fiber.StatusOK, unshared.status, unshared.body)
	assert.Equal(t, "Stopped sharing 1 clip", unshared.report.Message)
	assert.False(t, handler.clipQueue.GetJob("a").Shared)
}

// TestBulkRegeneratesWithinTheMembersLimits covers a member regenerating
// three clips at a limit of one concurrent render.
//
// The first is queued, and the other two are left alone with the limit
// named.
func TestBulkRegeneratesWithinTheMembersLimits(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)

	require.NoError(
		t,
		handler.db.SaveGlobalLimits(t.Context(), quota.Limits{Active: 1, Weekly: 10}),
	)
	handler.SetLimits(quota.New(handler.db, handler.clipQueue.CountActive))

	for _, id := range []string{"a", "b", "c"} {
		storeOwned(t, handler, source, id, middlewaretest.MemberID, clipdom.StatusCompleted)
	}

	answer := bulkJSON(t, handler, middlewaretest.AsMember(), "regenerate", "a", "b", "c")

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, 1, answer.report.Done)
	assert.Equal(t, 2, answer.report.NotStarted)
	assert.Equal(
		t,
		"Regenerated 1 clip. 2 clips weren't started. "+
			"You've reached your limit of 1 concurrent render. Wait for one to finish.",
		answer.report.Message,
	)
	assert.Equal(t, clipdom.StatusPending, handler.clipQueue.GetJob("a").Status)

	stored, err := handler.db.GetClip(t.Context(), "b")
	require.NoError(t, err)
	assert.Equal(t, clipdom.StatusCompleted, stored.Status, "a clip past the limit is left alone")
}

// TestBulkSkipsARegenerateOfAClipThatRenders covers Regenerate on a clip
// that is rendering, which is skipped rather than failed.
func TestBulkSkipsARegenerateOfAClipThatRenders(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)
	storeOwned(t, handler, source, "busy", middlewaretest.OwnerID, clipdom.StatusPending)

	taken := make(chan struct{})
	release := make(chan struct{})
	work := queue.NewQueue(1, func(context.Context, *clipdom.Job) error {
		close(taken)
		<-release

		return nil
	})

	t.Cleanup(func() {
		close(release)
		work.Stop()
	})

	busy, err := handler.db.GetClip(t.Context(), "busy")
	require.NoError(t, err)
	require.NoError(t, work.Submit(busy))

	handler.clipQueue = work

	work.Start(t.Context())
	<-taken

	answer := bulkJSON(t, handler, middlewaretest.AsOwner(), "regenerate", "busy")

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, 1, answer.report.Skipped)
	assert.Equal(t, "1 clip was skipped", answer.report.Message)
}

// TestBulkStopsOnceTheRequestEnds covers a request that ends before its
// clips are reached.
//
// No clip is changed, and every one counts as not started.
func TestBulkStopsOnceTheRequestEnds(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)
	storeOwned(t, handler, source, "a", middlewaretest.OwnerID, clipdom.StatusCompleted)
	storeOwned(t, handler, source, "b", middlewaretest.OwnerID, clipdom.StatusCompleted)

	ended := func(ctx fiber.Ctx) error {
		middleware.SetCaller(ctx, identity.User{
			PlexID:   middlewaretest.OwnerID,
			Username: "owner",
			Role:     identity.RoleOwner,
		})

		gone, cancel := context.WithCancel(ctx.Context())
		cancel()
		ctx.SetContext(gone)

		return ctx.Next()
	}

	answer := bulkJSON(t, handler, ended, "delete", "a", "b")

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, 0, answer.report.Done)
	assert.Equal(t, 2, answer.report.NotStarted)

	stored, err := handler.db.ListClips(t.Context())
	require.NoError(t, err)
	assert.Len(t, stored, 2, "nothing was deleted")
}

// TestBulkRefusesARequestTheListsNeverSend covers an unknown action, no
// clip, and more clips than one request may name.
func TestBulkRefusesARequestTheListsNeverSend(t *testing.T) {
	t.Parallel()

	handler, _ := sharedSourceHandler(t)
	many := make([]string, 0, maxBulkClips+1)

	for index := range maxBulkClips + 1 {
		many = append(many, strings.Repeat("x", index+1))
	}

	for name, test := range map[string]struct {
		action string
		ids    []string
		want   bulkRequestError
	}{
		"unknown action": {action: "explode", ids: []string{"a"}, want: errBulkAction},
		"no clip":        {action: "delete", ids: nil, want: errBulkNone},
		"too many":       {action: "delete", ids: many, want: errBulkTooMany},
	} {
		answer := bulkJSON(t, handler, middlewaretest.AsOwner(), test.action, test.ids...)

		assert.Equal(t, fiber.StatusBadRequest, answer.status, name)
		assert.Contains(t, answer.body, string(test.want), name)
	}
}

// TestBulkRefusesABodyItCannotRead covers a JSON body that does not parse,
// which is refused as unreadable rather than as naming no clip.
func TestBulkRefusesABodyItCannotRead(t *testing.T) {
	t.Parallel()

	handler, _ := sharedSourceHandler(t)

	answer := postBulk(t, handler, bulkCall{
		caller:      middlewaretest.AsOwner(),
		path:        "/api/clips/bulk",
		body:        `{"action":`,
		contentType: fiber.MIMEApplicationJSON,
		htmx:        false,
	})

	assert.Equal(t, fiber.StatusBadRequest, answer.status)
	assert.Contains(t, answer.body, string(errBulkUnreadable))
}

// TestBulkAnswersHTMXWithTheStatusLine covers the list's own form post,
// which names each clip in a repeated field and gets the status line back.
func TestBulkAnswersHTMXWithTheStatusLine(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)
	storeOwned(t, handler, source, "a", middlewaretest.OwnerID, clipdom.StatusCompleted)
	storeOwned(t, handler, source, "b", middlewaretest.OwnerID, clipdom.StatusCompleted)

	answer := postBulk(t, handler, bulkCall{
		caller:      middlewaretest.AsOwner(),
		path:        "/api/clips/bulk",
		body:        "action=share&id=a&id=b",
		contentType: fiber.MIMEApplicationForm,
		htmx:        true,
	})

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, "Shared 2 clips", strings.TrimSpace(answer.body))
}

// TestRetryFailedRegeneratesTheFailedClips covers Retry Failed, on a title's
// page and on the clips page.
//
// A title's page renders only that title's failed clips, the clips page every
// failed clip, and neither touches a finished clip.
func TestRetryFailedRegeneratesTheFailedClips(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)
	storeOwned(t, handler, source, "failed-here", middlewaretest.OwnerID, clipdom.StatusFailed)
	storeOwned(t, handler, source, "finished", middlewaretest.OwnerID, clipdom.StatusCompleted)

	elsewhere := testClipJob("failed-there", clipdom.TypeClip)

	elsewhere.MediaID = "99"
	elsewhere.InputPath = source
	elsewhere.OwnerID = middlewaretest.OwnerID
	elsewhere.Status = clipdom.StatusFailed
	elsewhere.StartTime = time.Second
	elsewhere.Duration = 5 * time.Second
	require.NoError(t, handler.db.SaveClip(t.Context(), elsewhere))

	// The app reloads every stored clip into the queue before it serves.
	stored, err := handler.db.ListClips(t.Context())
	require.NoError(t, err)

	for _, job := range stored {
		handler.clipQueue.Restore(job)
	}

	title := postBulk(t, handler, bulkCall{
		caller:      middlewaretest.AsOwner(),
		path:        "/api/clips/retry",
		body:        "mediaId=42",
		contentType: fiber.MIMEApplicationForm,
		htmx:        false,
	})

	require.Equal(t, fiber.StatusOK, title.status, title.body)
	assert.Equal(t, 1, title.report.Done)
	assert.Equal(t, clipdom.StatusPending, handler.clipQueue.GetJob("failed-here").Status)
	assert.Equal(t, clipdom.StatusFailed, handler.clipQueue.GetJob("failed-there").Status,
		"another title's clip is left alone")

	every := postBulk(t, handler, bulkCall{
		caller:      middlewaretest.AsOwner(),
		path:        "/api/clips/retry",
		body:        "",
		contentType: fiber.MIMEApplicationForm,
		htmx:        false,
	})

	require.Equal(t, fiber.StatusOK, every.status, every.body)
	assert.Equal(t, 1, every.report.Done, "the clip already retried is no longer failed")
	assert.Equal(t, clipdom.StatusPending, handler.clipQueue.GetJob("failed-there").Status)
}

// TestBulkRegeneratesAWaitingClipAtTheMembersLimit covers a member at their
// limit of concurrent renders regenerating a clip still waiting.
//
// The queue already counts it, so the regenerate is let through, and nothing
// is logged against the week.
func TestBulkRegeneratesAWaitingClipAtTheMembersLimit(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)

	require.NoError(
		t,
		handler.db.SaveGlobalLimits(t.Context(), quota.Limits{Active: 1, Weekly: 10}),
	)
	handler.SetLimits(quota.New(handler.db, handler.clipQueue.CountActive))

	storeOwned(t, handler, source, "waiting", middlewaretest.MemberID, clipdom.StatusPending)

	waiting, err := handler.db.GetClip(t.Context(), "waiting")
	require.NoError(t, err)
	require.NoError(t, handler.clipQueue.Submit(waiting))

	answer := bulkJSON(t, handler, middlewaretest.AsMember(), "regenerate", "waiting")

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, 1, answer.report.Done)
	assert.Zero(t, answer.report.NotStarted)

	started, _, err := handler.db.RendersSince(
		t.Context(), middlewaretest.MemberID, time.Now().Add(-time.Hour),
	)
	require.NoError(t, err)
	assert.Zero(t, started, "a clip already waiting is not a new render")
}

// TestRetryFailedRendersAtMostOneBatch covers more failed clips than one
// request may render.
//
// The newest maxBulkClips are rendered, and the rest wait for another click.
func TestRetryFailedRendersAtMostOneBatch(t *testing.T) {
	t.Parallel()

	handler, source := sharedSourceHandler(t)

	for index := range maxBulkClips + 3 {
		storeOwned(t, handler, source, fmt.Sprintf("failed-%03d", index),
			middlewaretest.OwnerID, clipdom.StatusFailed)
	}

	answer := postBulk(t, handler, bulkCall{
		caller:      middlewaretest.AsOwner(),
		path:        "/api/clips/retry",
		body:        "",
		contentType: fiber.MIMEApplicationForm,
		htmx:        false,
	})

	require.Equal(t, fiber.StatusOK, answer.status, answer.body)
	assert.Equal(t, maxBulkClips, answer.report.Done)
}
