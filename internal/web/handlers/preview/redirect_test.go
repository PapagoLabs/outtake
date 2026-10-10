// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

func TestPreviewRedirectKeepsMillisecondMarks(t *testing.T) {
	t.Parallel()

	for _, seconds := range []float64{10, 30.45, 99.999} {
		t.Run(strconv.FormatFloat(seconds, 'f', -1, 64), func(t *testing.T) {
			t.Parallel()

			end := seconds + 15

			target, parsed := previewRedirectParts(
				t,
				previewRedirect(queryPreview(seconds, end), "preview-1"),
			)

			assert.Equal(t, "/media/item/42", target,
				"the page reloads the same source")
			assert.Equal(t, "preview-1", parsed.Get("preview"), "the page polls this render")
			assert.Equal(t,
				timecode.FormatSeconds(seconds),
				parsed.Get("start"),
				"a three decimal mark survives the round trip",
			)
			assert.Equal(t,
				timecode.FormatSeconds(end),
				parsed.Get("end"),
				"the end mark is what the user is looking at",
			)
		})
	}
}

func TestPreviewRedirectMarksParseBack(t *testing.T) {
	t.Parallel()

	const seconds = 45.678

	_, parsed := previewRedirectParts(
		t,
		previewRedirect(queryPreview(seconds, seconds+10), "preview-1"),
	)

	start, err := timecode.Parse(parsed.Get(routes.QueryStart))
	require.NoError(t, err, "what the redirect writes the page has to read back")

	end, err := timecode.Parse(parsed.Get(routes.QueryEnd))
	require.NoError(t, err)

	assert.InDelta(t, seconds, start.Duration().Seconds(), 0.0005)
	assert.InDelta(t, 10, (end.Duration() - start.Duration()).Seconds(), 0.0005,
		"the length the page offers is the length the user marked")
}

func TestPreviewRedirectCarriesAnAbsentMark(t *testing.T) {
	t.Parallel()

	_, parsed := previewRedirectParts(t, previewRedirect(queryPreview(0, 0), "preview-1"))

	assert.Equal(t, "0.000", parsed.Get(routes.QueryStart))
	assert.Equal(t, "0.000", parsed.Get(routes.QueryEnd))
}

// previewRedirectParts reads a preview redirect apart into its path and query.
//
// Parameters:
//   - t: The test the redirect belongs to.
//   - target: Redirect location.
//
// Returns:
//   - path: The page the redirect returns to.
//   - query: The query the redirect carries.
func previewRedirectParts(t *testing.T, target string) (string, url.Values) {
	t.Helper()

	parsed, err := url.Parse(target)
	require.NoError(t, err)

	return parsed.Path, parsed.Query()
}

// queryPreview is the request a preview redirect carries for one window.
//
// Parameters:
//   - start: Where the window begins in the source.
//   - end: Where the window ends in the source.
//   - extra: Further query values the form submitted, as name=value pairs.
//
// Returns:
//   - req: The request the redirect is built from.
func queryPreview(start, end float64, extra ...string) api.ClipRequest {
	req := api.ClipRequest{
		MediaID:   "42",
		StartTime: start,
		Duration:  end - start,
	}

	fields := url.Values{}

	for _, pair := range extra {
		name, value, _ := strings.Cut(pair, "=")
		fields.Set(name, value)
	}

	req.PreserveHDR = new(fields.Get("preserveHdr") == routes.FormChecked)

	return req
}

// TestPreviewRedirectCarriesNoHDRChoice covers the redirect back to the form:
// HDR is the profile's setting, so the form gets no HDR choice back.
func TestPreviewRedirectCarriesNoHDRChoice(t *testing.T) {
	t.Parallel()

	_, query := previewRedirectParts(
		t, previewRedirect(queryPreview(0, 0, "preserveHdr=1"), "preview-1"),
	)

	assert.NotContains(t, query, "preserveHdr")
}

// TestScreenShowsHDRReadsTheFormOrTheQuery covers the screen flag: the export
// form posts it, an API caller may pass it in the query, and without it the
// screen is taken to be SDR.
func TestScreenShowsHDRReadsTheFormOrTheQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		form   string
		want   bool
	}{
		{name: "form field", target: "/x", form: "screenHdr=1", want: true},
		{name: "query", target: "/x?screenHdr=1", form: "", want: true},
		{name: "marked SDR", target: "/x", form: "screenHdr=0", want: false},
		{name: "absent", target: "/x", form: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got bool

			app := fiber.New()
			app.Post("/x", func(ctx fiber.Ctx) error {
				got = screenShowsHDR(ctx)

				return nil
			})

			req := httptest.NewRequestWithContext(
				t.Context(), http.MethodPost, test.target, strings.NewReader(test.form),
			)
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

			resp, err := app.Test(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())

			assert.Equal(t, test.want, got)
		})
	}
}

// TestPreviewRenderToneMapsHDRForAnSDRScreen covers what a preview renders:
// an HDR clip that keeps HDR is tone mapped for an SDR screen and kept for an
// HDR one, a clip that converts is always converted, an SDR source is never
// marked as shown in SDR, and the request keeps the clip's own choice.
func TestPreviewRenderToneMapsHDRForAnSDRScreen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		keep      bool
		sourceHDR bool
		screenHDR bool
		renders   bool
	}{
		{
			name:      "HDR clip on an SDR screen",
			keep:      true,
			sourceHDR: true,
			screenHDR: false,
			renders:   false,
		},
		{
			name:      "HDR clip on an HDR screen",
			keep:      true,
			sourceHDR: true,
			screenHDR: true,
			renders:   true,
		},
		{
			name:      "converted clip on an HDR screen",
			keep:      false,
			sourceHDR: true,
			screenHDR: true,
			renders:   false,
		},
		{
			name:      "converted clip on an SDR screen",
			keep:      false,
			sourceHDR: true,
			screenHDR: false,
			renders:   false,
		},
		{
			name:      "SDR source under a keep-HDR profile",
			keep:      true,
			sourceHDR: false,
			screenHDR: false,
			renders:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := api.ClipRequest{MediaID: "42", PreserveHDR: new(test.keep)}

			render := previewRender(req, test.sourceHDR, test.screenHDR)

			assert.Equal(t, test.renders, *render.PreserveHDR)
			assert.Equal(t, test.keep, *req.PreserveHDR, "the clip's choice is untouched")
		})
	}
}
