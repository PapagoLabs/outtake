// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/queue"
)

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
