// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestPreviewRedirectCarriesTheWebSafeColorToggle(t *testing.T) {
	t.Parallel()

	_, checked := previewRedirectParts(
		t, previewRedirect(queryPreview(0, 0, "webSafeColor=1"), "preview-1"),
	)

	assert.Equal(t, routes.FormChecked, checked.Get(routes.QueryWebSafeColor))

	_, absent := previewRedirectParts(t, previewRedirect(queryPreview(0, 0), "preview-1"))

	assert.Equal(t, routes.FormUnchecked, absent.Get(routes.QueryWebSafeColor),
		"an absent setting is carried as an unchecked box, so the page does not fall back")
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

	req.WebSafeColor = new(fields.Get(routes.QueryWebSafeColor) == routes.FormChecked)

	return req
}
