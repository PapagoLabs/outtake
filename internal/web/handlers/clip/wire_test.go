// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
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
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

func parseForm(t *testing.T, form url.Values) api.ClipRequest {
	t.Helper()

	req, err := parseFormResult(t, form)
	require.NoError(t, err, "a form post is always parseable")

	return req
}

func parseFormResult(t *testing.T, form url.Values) (api.ClipRequest, error) {
	t.Helper()

	app := fiber.New()

	var (
		gotReq api.ClipRequest
		gotErr error
	)

	app.Post("/api/clips", func(ctx fiber.Ctx) error {
		gotReq, gotErr = ParseRequest(ctx)

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

	//nolint:wrapcheck // The parser's own error carries the field and a test reads it back.
	return gotReq, gotErr
}

func markForm(start, end string) url.Values {
	return url.Values{
		"mediaId":    {"42"},
		"mediaTitle": {"Movie"},
		"startTime":  {start},
		"endTime":    {end},
		"clipType":   {string(clipdom.TypeClip)},
	}
}

func parseJSON(t *testing.T, body string) api.ClipRequest {
	t.Helper()

	app := fiber.New()

	var gotReq api.ClipRequest

	app.Post("/api/clips", func(ctx fiber.Ctx) error {
		parsed, err := ParseRequest(ctx)
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

func TestParseClipRequestDerivesDurationFromTheMarks(t *testing.T) {
	t.Parallel()

	req := parseForm(t, markForm("00:00:10.000", "00:00:25.000"))

	assert.InDelta(t, 15, req.Duration, 0.001)
	assert.InDelta(t, 10, req.StartTime, 0.001)
}

func TestParseRequestKeepsFormValuesAfterTheRequestBufferIsReused(t *testing.T) {
	t.Parallel()

	const title = "Movie Title"

	form := url.Values{
		"name":       {title},
		"mediaId":    {"42"},
		"mediaTitle": {title},
		"mediaType":  {"movie"},
		"startTime":  {"00:00:10.000"},
		"endTime":    {"00:00:25.000"},
		"quality":    {"archive"},
		"clipType":   {string(clipdom.TypeClip)},
	}

	app := fiber.New()

	var parsed api.ClipRequest

	app.Post("/api/clips", func(ctx fiber.Ctx) error {
		var err error

		parsed, err = ParseRequest(ctx)
		require.NoError(t, err)

		overwriteFormValue(ctx, "mediaTitle", "clip")
		overwriteFormValue(ctx, "name", "clip")
		overwriteFormValue(ctx, "mediaType", "show")
		overwriteFormValue(ctx, "mediaId", "99")
		overwriteFormValue(ctx, "quality", "low")
		overwriteFormValue(ctx, "clipType", "gif")

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

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, title, parsed.Name)
	assert.Equal(t, title, parsed.MediaTitle)
	assert.Equal(t, "movie", parsed.MediaType)
	assert.Equal(t, "42", parsed.MediaID)
	assert.Equal(t, "archive", parsed.Quality)
	assert.Equal(t, string(clipdom.TypeClip), parsed.ClipType)
}

// overwriteFormValue stomps a parsed form field the way a reused request buffer does.
//
// Parameters:
//   - ctx: Request whose form buffer is still live.
//   - key: Field to overwrite.
//   - replacement: Bytes written over the start of the field.
func overwriteFormValue(ctx fiber.Ctx, key, replacement string) {
	buf := ctx.Request().PostArgs().Peek(key)
	if len(buf) < len(replacement) {
		return
	}

	copy(buf[:len(replacement)], replacement)
}

func TestParseClipRequestIgnoresAStaleDurationField(t *testing.T) {
	t.Parallel()

	form := markForm("00:00:10.000", "00:00:25.000")
	form.Set("duration", "600")

	req := parseForm(t, form)

	assert.InDelta(t, 15, req.Duration, 0.001,
		"a stale duration field must not decide the length of the clip")
}

func TestParseClipRequestKeepsMillisecondPrecision(t *testing.T) {
	t.Parallel()

	req := parseForm(t, markForm("00:01:02.345", "00:01:12.678"))

	assert.InDelta(t, 10.333, req.Duration, 0.0005)
}

func TestParseClipRequestRejectsAnInvertedRange(t *testing.T) {
	t.Parallel()

	form := markForm("00:00:25.000", "00:00:10.000")
	form.Set("duration", "15")

	req := parseForm(t, form)

	assert.Zero(t, req.Duration, "an end before the start must not produce a duration")
}

func TestParseClipRequestRejectsAMalformedMark(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		clipType string
		form     url.Values
	}{
		{
			name:     "a bare minutes prefix the browser reads as ten minutes",
			clipType: string(clipdom.TypeClip),
			form:     markForm("10:", "00:00:25.000"),
		},
		{
			name:     "letters in the seconds field",
			clipType: string(clipdom.TypeClip),
			form:     markForm("00:00:10.000", "00:00:ab.000"),
		},
		{
			name:     "four colon separated fields",
			clipType: string(clipdom.TypeClip),
			form:     markForm("00:00:00:10.000", "00:00:25.000"),
		},
		{
			name:     "non numeric minutes",
			clipType: string(clipdom.TypeClip),
			form:     markForm("00:aa:10.000", "00:00:25.000"),
		},
		{
			name:     "two fractional separators",
			clipType: string(clipdom.TypeClip),
			form:     markForm("00:00:10.000.000", "00:00:25.000"),
		},
		{
			name:     "a bare word",
			clipType: string(clipdom.TypeClip),
			form:     markForm("start", "00:00:25.000"),
		},
		{
			name:     "a start of spaces only",
			clipType: string(clipdom.TypeClip),
			form:     markForm("   ", "00:00:25.000"),
		},
		{
			name:     "an end of spaces only",
			clipType: string(clipdom.TypeClip),
			form:     markForm("00:00:10.000", "\t"),
		},
		{
			name:     "a malformed mark on a screenshot",
			clipType: string(clipdom.TypeScreenshot),
			form:     markForm("00:00:10.000", "00:00:1a.000"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.form.Set("clipType", test.clipType)

			_, err := parseFormResult(t, test.form)
			require.Error(t, err, "a mark that is not a timecode is not a position")
			require.ErrorIs(t, err, timecode.ErrInvalidTimecode)
			assert.Contains(t, err.Error(), "timecode",
				"the reason names what the field wanted")
		})
	}
}

func TestParseClipRequestAcceptsAnAbsentMark(t *testing.T) {
	t.Parallel()

	req := parseForm(t, markForm("", ""))

	assert.Zero(t, req.StartTime)
	assert.Zero(t, req.Duration)
}

func TestParseClipRequestAcceptsAPaddedMark(t *testing.T) {
	t.Parallel()

	req := parseForm(t, markForm(" 00:00:10.000 ", "\t00:00:25.000\t"))

	assert.InDelta(t, 15, req.Duration, 0.001)
	assert.InDelta(t, 10, req.StartTime, 0.001)
}

func TestParseClipRequestRejectsAMalformedStartAndEndSeparately(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		form  url.Values
		which string
	}{
		{
			name:  "the start is named when the start is malformed",
			form:  markForm("nonsense", "00:00:25.000"),
			which: "the start",
		},
		{
			name:  "the end is named when the end is malformed",
			form:  markForm("00:00:10.000", "nonsense"),
			which: "the end",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseFormResult(t, test.form)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.which)
		})
	}
}

func TestParseClipRequestKeepsMillisecondPrecisionThroughTheEdit(t *testing.T) {
	t.Parallel()

	for ms := range 3000 {
		start := float64(ms) / 1000
		form := markForm(
			timecode.FromSeconds(start).String(),
			timecode.FromSeconds(start+600).String(),
		)

		edit := clipEdit(parseForm(t, form))

		assert.Equal(t, time.Duration(ms)*time.Millisecond, edit.Start.Round(time.Microsecond),
			"the start must survive to the microsecond, start %v", start)
		assert.Equal(t, 600*time.Second, edit.Length,
			"an exactly maximum range must survive, start %v", start)
	}
}

// TestParseClipRequestTakesNoKeepHDRFromAForm covers Keep HDR as a profile
// setting: a form that still posts the old checkbox, checked or not, chooses
// nothing, and the clip takes its profile's setting.
func TestParseClipRequestTakesNoKeepHDRFromAForm(t *testing.T) {
	t.Parallel()

	for _, posted := range []string{routes.FormChecked, routes.FormUnchecked} {
		form := markForm("10", "15")
		form.Set("preserveHdr", posted)

		got := parseForm(t, form)

		assert.Nil(t, got.PreserveHDR, "posted %q", posted)
		assert.Nil(t, got.WebSafeColor)
	}
}

// TestParseClipRequestRefusesAKeepHDRChoiceFromAnAPIClient covers a JSON
// caller that still chooses HDR: it is refused rather than given the
// profile's file in silence, whichever field and value it sends.
func TestParseClipRequestRefusesAKeepHDRChoiceFromAnAPIClient(t *testing.T) {
	t.Parallel()

	for _, field := range []string{`"preserveHdr":true`, `"preserveHdr":false`, `"webSafeColor":true`} {
		app := fiber.New()

		var gotErr error

		app.Post("/api/clips", func(ctx fiber.Ctx) error {
			_, gotErr = ParseRequest(ctx)

			return ctx.SendStatus(fiber.StatusOK)
		})

		post := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/api/clips",
			strings.NewReader(`{"mediaId":"42","startTime":10,"duration":15,`+field+`}`),
		)
		post.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

		resp, err := app.Test(post)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())

		assert.ErrorIs(t, gotErr, clipdom.ErrHDRIsAProfileSetting, field)
	}
}

func TestAnAPIClientCarriesTheSameMarksAsTheForm(t *testing.T) {
	t.Parallel()

	req := parseJSON(t, `{"mediaId":"42","clipType":"clip","startTime":10,"duration":15}`)

	edit := clipEdit(req)

	assert.Equal(t, 10*time.Second, edit.Start)
	assert.Equal(t, 15*time.Second, edit.Length)
}

func TestClipEditCarriesTheRequest(t *testing.T) {
	t.Parallel()

	req := api.ClipRequest{
		Name:          "Intro",
		Quality:       "archive",
		StartTime:     12.5,
		Duration:      5.25,
		Width:         1280,
		FPS:           24,
		AudioIndex:    3,
		CropBlackBars: true,
	}

	edit := clipEdit(req)

	assert.Equal(t, "Intro", edit.Name)
	assert.Equal(t, "archive", edit.Quality)
	assert.Equal(t, 12500*time.Millisecond, edit.Start)
	assert.Equal(t, 5250*time.Millisecond, edit.Length)
	assert.Equal(t, 1280, edit.Width)
	assert.Equal(t, 24, edit.FPS)
	assert.Equal(t, 3, edit.AudioIndex)
	assert.True(t, edit.CropBlackBars)
	assert.Nil(t, edit.PreserveHDR, "Keep HDR is the profile's, set where the clip renders")
	assert.Empty(t, edit.Type, "the type is decided by the caller, not by the wire request")
}

func TestRequestWindow(t *testing.T) {
	t.Parallel()

	start, length := requestWindow(api.ClipRequest{StartTime: 61.999, Duration: 75.001 - 61.999})

	assert.Equal(t, 61999*time.Millisecond, start)
	assert.Equal(t, 13002*time.Millisecond, length)

	absentStart, absentLength := requestWindow(api.ClipRequest{})

	assert.Zero(t, absentStart, "an absent start mark stays at the start of the source")
	assert.Zero(t, absentLength)
}

func TestClipReturnPath(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/media/item/42", ReturnPath("42"))
	assert.Equal(t, routes.PathClips, ReturnPath(""), "a post with no media goes back to the list")
}
