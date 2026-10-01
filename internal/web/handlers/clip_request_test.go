// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"fmt"
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
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/queue"
	"github.com/PapagoLabs/outtake/internal/storage"
)

// testMediaPath is the source path a stubbed probe reports a length for.
const testMediaPath = "/media/movie.mkv"

// parseForm posts form values through parseClipRequest and returns the result.
//
// Parameters:
//   - t: Test context.
//   - form: Form values to post.
//
// Returns:
//   - req: The parsed request.
func parseForm(t *testing.T, form url.Values) api.ClipRequest {
	t.Helper()

	app := fiber.New()

	var gotReq api.ClipRequest

	app.Post("/api/clips", func(ctx fiber.Ctx) error {
		parsed, err := parseClipRequest(ctx)
		require.NoError(t, err, "a form post is always parseable")

		gotReq = parsed

		return ctx.SendStatus(fiber.StatusOK)
	})

	post := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/clips",
		strings.NewReader(form.Encode()),
	)
	post.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	resp, err := app.Test(post)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return gotReq
}

// markForm is a minimal form body with the given marks.
func markForm(start, end string) url.Values {
	return url.Values{
		"mediaId":    {"42"},
		"mediaTitle": {testMovie},
		"startTime":  {start},
		"endTime":    {end},
		"clipType":   {"clip"},
	}
}

// TestParseClipRequestDerivesDurationFromTheMarks covers the duration coming
// from the marks the user typed.
//
// The browser also posts a hidden duration it computed, and that made a second
// source of truth which could disagree with the form on screen. When it did, the
// clip came out a different length to the one displayed.
func TestParseClipRequestDerivesDurationFromTheMarks(t *testing.T) {
	t.Parallel()

	req := parseForm(t, markForm("00:00:10.000", "00:00:25.000"))

	assert.InDelta(t, 15, req.Duration, 0.001)
	assert.InDelta(t, 10, req.StartTime, 0.001)
}

// TestParseClipRequestIgnoresAStaleDurationField is the regression guard for
// that second source of truth.
func TestParseClipRequestIgnoresAStaleDurationField(t *testing.T) {
	t.Parallel()

	form := markForm("00:00:10.000", "00:00:25.000")

	// What the page would have posted before the marks were edited.
	form.Set("duration", "600")

	req := parseForm(t, form)

	assert.InDelta(t, 15, req.Duration, 0.001,
		"a stale duration field must not decide the length of the clip")
}

// TestParseClipRequestKeepsMillisecondPrecision guards the marks surviving the
// round trip, which is what makes the derived duration exact.
func TestParseClipRequestKeepsMillisecondPrecision(t *testing.T) {
	t.Parallel()

	req := parseForm(t, markForm("00:01:02.345", "00:01:12.678"))

	// Half a millisecond, so a whole millisecond of loss is caught.
	assert.InDelta(t, 10.333, req.Duration, 0.0005)
}

// TestParseClipRequestRejectsAnInvertedRange covers an end that is not after the
// start. The duration stays zero so validation rejects it, rather than a stale
// posted value rescuing it.
func TestParseClipRequestRejectsAnInvertedRange(t *testing.T) {
	t.Parallel()

	form := markForm("00:00:25.000", "00:00:10.000")
	form.Set("duration", "15")

	req := parseForm(t, form)

	assert.Zero(t, req.Duration, "an end before the start must not produce a duration")
}

// TestValidateDurationDescribesTheRange is the diagnostic guard.
//
// A non-positive duration means two different things. A form post has no
// duration of its own, so it can only be an end that is not after the start. A
// JSON caller sends a duration directly and never sends an end at all, so the
// wording has to describe the range rather than name an end it may never have
// sent.
func TestValidateDurationDescribesTheRange(t *testing.T) {
	t.Parallel()

	handler := &ClipHandler{cfg: &config.Config{MaxClipDurSec: 600}}

	err := handler.validateDuration(queue.JobTypeClip, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "range must be longer than zero")
	assert.NotContains(t, err.Error(), "end must be after the start",
		"a JSON caller never sent an end, so the message must not assume one")

	// A screenshot takes no duration, so zero is correct there.
	require.NoError(t, handler.validateDuration(queue.JobTypeScreenshot, 0))

	// Over the limit is still reported as a limit.
	over := handler.validateDuration(queue.JobTypeClip, 601)
	require.Error(t, over)
	assert.Contains(t, over.Error(), "600")
}

// TestParseClipRequestAcceptsAnExactlyMaximumRange is the boundary invariant for
// a range measured from two marks.
//
// A selection of exactly the maximum length must parse and validate, whatever the
// marks it was built from. The range is subtracted as a duration, so this holds
// by construction rather than by the marks happening to be millisecond aligned.
func TestParseClipRequestAcceptsAnExactlyMaximumRange(t *testing.T) {
	t.Parallel()

	handler := &ClipHandler{cfg: &config.Config{MaxClipDurSec: 600}}

	// Every millisecond start whose subtraction lands a rounding error high, plus
	// the exact edges, must survive both parsing and validation.
	for ms := range 3000 {
		start := float64(ms) / 1000
		form := markForm(
			media.FromSeconds(start).String(),
			media.FromSeconds(start+600).String(),
		)

		req := parseForm(t, form)

		require.InDelta(t, 600, req.Duration, 0.0005,
			"an exactly maximum range must survive, start %v", start)
		require.NoError(t, handler.validateDuration(queue.JobTypeClip, req.Duration),
			"an exactly maximum range must not be rejected as too long, start %v", start)
	}
}

// TestParseClipRequestReadsTheKeepHDRCheckbox covers the per-clip preserve-HDR
// control.
//
// It is a checkbox, so an absent field and a present-but-false one are the same
// answer, and only a checked box may preserve the source.
func TestParseClipRequestReadsTheKeepHDRCheckbox(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		form url.Values
		want bool
	}{
		{
			name: "an omitted checkbox tone maps",
			form: markForm("10", "15"),
			want: false,
		},
		{
			name: "a checked checkbox preserves the source",
			form: func() url.Values {
				form := markForm("10", "15")
				form.Set("preserveHdr", formChecked)

				return form
			}(),
			want: true,
		},
		{
			name: "an unchecked checkbox tone maps",
			form: func() url.Values {
				form := markForm("10", "15")
				form.Set("preserveHdr", "0")

				return form
			}(),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := parseForm(t, test.form)

			require.NotNil(t, got.PreserveHDR,
				"the form always decides, so the value is never left to the server")
			assert.Equal(t, test.want, *got.PreserveHDR)
		})
	}
}

// TestPreserveHDRFor covers the documented fallback for an absent field.
//
// The field's documentation promises the server default when a request omits
// it, and an explicit false is the caller declining rather than an absent value.
func TestPreserveHDRFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested *bool
		fallback  bool
		want      bool
	}{
		{
			name:      "an absent field takes the configured default",
			requested: nil,
			fallback:  true,
			want:      true,
		},
		{
			name:      "an absent field with no default stays off",
			requested: nil,
			fallback:  false,
			want:      false,
		},
		{
			name:      "an explicit true is honored against a false default",
			requested: new(true),
			fallback:  false,
			want:      true,
		},
		{
			name:      "an explicit false is honored against a true default",
			requested: new(false),
			fallback:  true,
			want:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, preserveHDRFor(test.requested, test.fallback))
		})
	}
}

// TestCheckRange covers the bound the duration checks cannot catch.
//
// An 11 hour start on a 2 hour film is a legal clip length, so it passes every
// other bound and is persisted before ffmpeg reports that it seeked past the
// end. Only the source's own length rejects it.
func TestCheckRange(t *testing.T) {
	t.Parallel()

	const film = 2 * 60 * 60 // a two hour feature

	tests := []struct {
		name      string
		start     float64
		duration  float64
		media     float64
		wantError bool
	}{
		{name: "a range inside the film is accepted", start: 30, duration: 20, media: film},
		{
			name:     "a range ending exactly at the end is accepted",
			start:    100,
			duration: film - 100,
			media:    film,
		},
		{
			name:      "a start past the end is rejected",
			start:     11 * 3600,
			duration:  20,
			media:     film,
			wantError: true,
		},
		{
			name:      "a range reaching past the end is rejected",
			start:     3600,
			duration:  3601,
			media:     film,
			wantError: true,
		},
		{
			name:     "a start one second before the end is accepted",
			start:    film - 1,
			duration: 1,
			media:    film,
		},
		{
			name:      "a negative start is rejected",
			start:     -1,
			duration:  10,
			media:     film,
			wantError: true,
		},
		{
			name:      "a screenshot is bounded by its start alone",
			start:     film,
			duration:  0,
			media:     film,
			wantError: true,
		},
		{name: "an unprobed source is not rejected", start: 11 * 3600, duration: 20, media: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := checkRange(test.start, test.duration, test.media)

			if test.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errRangeOutsideMedia)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestCheckRangeNamesTheBounds keeps the diagnostic usable.
//
// The whole point of rejecting is that the user is told which mark was wrong and
// by how much, so the message has to name both times rather than only complain.
func TestCheckRangeNamesTheBounds(t *testing.T) {
	t.Parallel()

	err := checkRange(11*3600, 20, 2*3600)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "11hr", "the message names the start the user typed")
	assert.Contains(t, err.Error(), "2hr", "the message names the media's real length")
}

// TestApplyClipEditsKeepsARejectedSelectionOutOfTheRow covers the seam that
// turned a bad selection into saved metadata.
//
// A start of 11 hours on a 2 hour film is a legal clip length, so the duration
// checks accepted it, the row was written, and only the render then failed. The
// row outlived the failure with marks that describe nothing.
//
// The bound needs a probe, which a white-box test may not run, so this pins the
// ordering it depends on: checkRange rejects first, and the edits are never
// applied when it does.
func TestApplyClipEditsKeepsARejectedSelectionOutOfTheRow(t *testing.T) {
	t.Parallel()

	const film = 2 * 60 * 60

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	job := testClipJob("out-of-range", queue.JobTypeClip)

	job.StartTime = 30
	job.Duration = 20
	job.InputPath = testMediaPath
	require.NoError(t, db.SaveClip(t.Context(), job))

	// The probe is stubbed rather than run, since a white-box test may not reach
	// for ffprobe. The source is two hours long. The queue is only there for
	// lookupJob and is never asked to run a job, since the request is rejected
	// before anything is queued.
	handler := &ClipHandler{
		db:          db,
		cfg:         &config.Config{MaxClipDurSec: 600},
		clipQueue:   queue.NewQueue(1, nil),
		clipStorage: &storage.Storage{},
		mediaDurationFn: func(_ context.Context, _ string) (float64, bool) {
			return film, true
		},
	}

	app := fiber.New()
	app.Post("/api/clips/:id/update", handler.Update)

	form := url.Values{
		"mediaId":   {job.MediaID},
		"startTime": {"11:00:00.000"},
		"endTime":   {"11:00:20.000"},
		"quality":   {defaultQuality},
	}

	post := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/clips/"+job.ID+"/update",
		strings.NewReader(form.Encode()),
	)
	post.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	resp, err := app.Test(post)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	// A form post is redirected back to the media page with the reason in the
	// query, rather than being answered with a status the browser would render
	// as a bare error page.
	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Contains(t, resp.Header.Get(fiber.HeaderLocation), "error=",
		"the redirect carries the reason back to the form")

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)

	assert.InDelta(t, 30, stored.StartTime, 0.0005, "the stored start is untouched")
	assert.InDelta(t, 20, stored.Duration, 0.0005, "the stored length is untouched")
}

// TestUpdateRejectsAnOutOfRangeSelectionForAnAPIClient is the same rejection
// seen by a JSON caller, who gets a status rather than a redirect.
func TestUpdateRejectsAnOutOfRangeSelectionForAnAPIClient(t *testing.T) {
	t.Parallel()

	const film = 2 * 60 * 60

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	job := testClipJob("api-out-of-range", queue.JobTypeClip)

	job.StartTime = 30
	job.Duration = 20
	job.InputPath = testMediaPath
	require.NoError(t, db.SaveClip(t.Context(), job))

	handler := &ClipHandler{
		db:          db,
		cfg:         &config.Config{MaxClipDurSec: 600},
		clipQueue:   queue.NewQueue(1, nil),
		clipStorage: &storage.Storage{},
		mediaDurationFn: func(_ context.Context, _ string) (float64, bool) {
			return film, true
		},
	}

	app := fiber.New()
	app.Post("/api/clips/:id/update", handler.Update)

	post := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/clips/"+job.ID+"/update",
		strings.NewReader(`{"clipType":"clip","startTime":39600,"duration":20}`),
	)
	post.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(post)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode,
		"an API client is told the status rather than being redirected")

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)

	assert.InDelta(t, 30, stored.StartTime, 0.0005, "the stored start is untouched")
	assert.InDelta(t, 20, stored.Duration, 0.0005, "the stored length is untouched")
}

// TestCheckRangeAcceptsWhatApplyClipEditsWouldStore is the other half: a
// selection the gate accepts is one the row can legitimately carry.
func TestCheckRangeAcceptsWhatApplyClipEditsWouldStore(t *testing.T) {
	t.Parallel()

	const film = 2 * 60 * 60

	job := testClipJob("in-range", queue.JobTypeClip)
	req := api.ClipRequest{ClipType: clipTypeClip, StartTime: 3600, Duration: 600}

	require.NoError(t, checkRange(req.StartTime, req.Duration, film))

	applyClipEdits(job, req)

	assert.InDelta(t, req.StartTime, job.StartTime, 0.0005)
	assert.InDelta(t, req.Duration, job.Duration, 0.0005)
}

// TestValidateSelectionIgnoresAnEndAScreenshotNeverUses covers the false
// rejection this bound would otherwise cause.
//
// ExtractScreenshot seeks to StartTime and nothing else, but parseClipRequest
// still derives a duration from the two marks, and validateDuration accepts it
// for a screenshot. Without the type check, a frame at 8000s of an 8013s film is
// rejected because the end mark beside it reaches past the source.
func TestValidateSelectionIgnoresAnEndAScreenshotNeverUses(t *testing.T) {
	t.Parallel()

	const film = 8013.846 // the runtime this was reported against

	// A frame four seconds from the end, with an end mark ten seconds past it.
	req := api.ClipRequest{
		ClipType:  "screenshot",
		StartTime: 8000,
		Duration:  23.846,
	}

	// The raw range is what a clip would be judged on, and it is out of bounds.
	require.Error(t,
		checkRange(req.StartTime, req.Duration, film),
		"a range reaching past the end is out of bounds for a clip",
	)

	// A screenshot is bounded by its start alone, so the same marks are fine.
	// selectionDuration is what decides that, so it is called rather than a
	// hand-picked zero, and dropping its screenshot case fails this test.
	require.Zero(t, selectionDuration(queue.JobTypeScreenshot, req),
		"a screenshot renders one frame, so it has no range to bound")
	require.NoError(t,
		checkRange(req.StartTime, selectionDuration(queue.JobTypeScreenshot, req), film),
		"a screenshot is a single frame at the start, so the end mark is not its concern")
}

// TestSelectionDurationKeepsTheRangeForEveryOtherType guards the other
// direction: the relaxation is for screenshots only.
func TestSelectionDurationKeepsTheRangeForEveryOtherType(t *testing.T) {
	t.Parallel()

	const film = 8013.846

	req := api.ClipRequest{StartTime: 8000, Duration: 23.846}

	for _, jobType := range []queue.JobType{queue.JobTypeClip, queue.JobTypeGIF} {
		assert.InDelta(t, req.Duration, selectionDuration(jobType, req), 0.0005,
			"%s renders a range, so it is still bounded", jobType)
		require.Error(t, checkRange(8000, selectionDuration(jobType, req), film),
			"a clip and a GIF both render a range, so the end is still bounded")
	}
}

// parseJSON posts a JSON body through parseClipRequest, the way an API client
// would rather than a browser form.
//
// Parameters:
//   - t: Test context.
//   - body: Request payload.
//
// Returns:
//   - req: The parsed request.
func parseJSON(t *testing.T, body string) api.ClipRequest {
	t.Helper()

	app := fiber.New()

	var gotReq api.ClipRequest

	app.Post("/api/clips", func(ctx fiber.Ctx) error {
		parsed, err := parseClipRequest(ctx)
		if err != nil {
			return ctx.SendStatus(fiber.StatusBadRequest)
		}

		gotReq = parsed

		return ctx.SendStatus(fiber.StatusOK)
	})

	post := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/clips",
		strings.NewReader(body),
	)
	post.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(post)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return gotReq
}

// TestAnAPIClientGetsTheSameBoundsAsTheForm pins that the bounds are a property
// of the request, not of the form that produced it.
//
// The form validates in the browser and blocks its own buttons, but a JSON
// client sends the same fields with no form and no script, so the server has to
// hold the line on its own.
func TestAnAPIClientGetsTheSameBoundsAsTheForm(t *testing.T) {
	t.Parallel()

	const film = 2 * 60 * 60

	tests := []struct {
		name      string
		start     float64
		duration  float64
		wantError bool
	}{
		{name: "a range inside the film is accepted", start: 30, duration: 20},
		{name: "a start past the end is rejected", start: 11 * 3600, duration: 20, wantError: true},
		{
			name:      "a range reaching past the end is rejected",
			start:     3600,
			duration:  3601,
			wantError: true,
		},
		{name: "a zero length range is rejected", start: 30, duration: 0, wantError: true},
		{name: "a negative start is rejected", start: -5, duration: 20, wantError: true},
		{name: "a range over the maximum is rejected", start: 0, duration: film, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			body := fmt.Sprintf(
				`{"mediaId":"42","clipType":"clip","startTime":%v,"duration":%v}`,
				test.start,
				test.duration,
			)

			req := parseJSON(t, body)

			// The parse succeeds either way, since binding is not validation.
			// What matters is that both bounds are then applied to what it parsed:
			// the length cap from validateDuration, and the source's own length
			// from checkRange. That is the pair validateNewClip runs.
			handler := &ClipHandler{cfg: &config.Config{MaxClipDurSec: 600}}

			err := handler.validateDuration(queue.JobTypeClip, req.Duration)
			if err == nil {
				err = checkRange(req.StartTime, req.Duration, film)
			}

			if test.wantError {
				require.Error(t, err, "the server must reject %s", body)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestQueueRegenerateAfterACancel covers the sequence a clip gets stuck on: start
// a render, cancel it without deleting the clip, then regenerate.
//
// The regenerate resets the job's status to pending, which is the very state a
// duplicate check reads. Doing the reset before the check made the clip look
// like a second job for its own id, so it was refused while the row had already
// been written saying pending with nothing running it. Restarting the process
// appeared to fix it, because a fresh queue holds no jobs for the check to find.
//
// Parameters:
//   - t: Test context.
func TestQueueRegenerateAfterACancel(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	settled := make(chan struct{}, 1)

	jobQueue := queue.NewQueue(1, func(_ context.Context, _ *queue.Job) error {
		return nil
	})
	// Persist on every status change, the way the real handler does, so the
	// end state in the row is what the user would see.
	jobQueue.SetStatusFunc(func(job *queue.Job) {
		_ = db.SaveClip(t.Context(), job)

		if job.Status == queue.JobStatusCompleted {
			settled <- struct{}{}
		}
	})
	jobQueue.Start(t.Context())

	t.Cleanup(jobQueue.Stop)

	handler := &ClipHandler{
		clipQueue:   jobQueue,
		clipStorage: &storage.Storage{},
		db:          db,
		cfg:         &config.Config{MaxClipDurSec: 600},
	}

	job := &queue.Job{
		ID:         "cancel-then-regenerate",
		Type:       queue.JobTypeClip,
		MediaID:    "100",
		MediaTitle: testMovie,
		MediaType:  defaultMediaType,
		InputPath:  testMediaPath,
		Quality:    defaultQuality,
		Status:     queue.JobStatusPending,
	}
	require.NoError(t, db.SaveClip(t.Context(), job))
	jobQueue.Restore(job)

	// Cancel without deleting, which is what leaves the job in the map, idle.
	require.True(t, jobQueue.Cancel(job.ID), "the render is canceled")
	require.False(t, jobQueue.Cancel(job.ID), "and canceling it twice changes nothing")

	require.NoError(t, handler.queueRegenerate(job),
		"a canceled clip is idle, so it can be run again")

	select {
	case <-settled:
	case <-time.After(2 * time.Second):
		t.Fatal("the regenerated job was queued but never finished")
	}

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, queue.JobStatusCompleted, stored.Status,
		"the row ends where the render does, rather than pending with nothing running it")
}
