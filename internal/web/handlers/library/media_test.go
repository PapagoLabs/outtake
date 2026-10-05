// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestChooserLibraries(t *testing.T) {
	t.Parallel()

	libs := []view.LibraryItem{{ID: "1", Title: "Movies", Type: clip.DefaultMediaType}}

	assert.Equal(t, libs, chooserLibraries(libs, "", ""),
		"an unfiltered chooser offers every library")
	assert.Nil(t, chooserLibraries(libs, "", "1"),
		"a library that is already chosen is not offered again")
	assert.Nil(t, chooserLibraries(libs, "query", ""),
		"a search in progress offers no library to jump to")
	assert.Nil(t, chooserLibraries(libs, "query", "1"))
}

func TestWantsMediaResults(t *testing.T) {
	t.Parallel()

	const (
		wantPage     = "page"
		wantFragment = "fragment"
	)

	tests := []struct {
		giveTarget string
		want       string
	}{
		{giveTarget: "", want: wantPage},
		{giveTarget: "main-content", want: wantPage},
		{giveTarget: "main#main-content", want: wantPage},
		{giveTarget: "media-browse", want: wantFragment},
		{giveTarget: "div#media-browse", want: wantFragment},
		{giveTarget: "media-results", want: wantPage},
	}

	for _, test := range tests {
		t.Run(test.giveTarget, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, mediaResultsKind(t, test.giveTarget))
		})
	}
}

func TestWantsMediaMore(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "page", mediaMoreKind(t, ""))
	assert.Equal(t, "page", mediaMoreKind(t, "media-browse"))
	assert.Equal(t, "more", mediaMoreKind(t, "media-more"))
	assert.Equal(t, "more", mediaMoreKind(t, "div#media-more"))
}

func TestWantsMediaPrev(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "page", mediaPrevKind(t, ""))
	assert.Equal(t, "page", mediaPrevKind(t, "media-browse"))
	assert.Equal(t, "prev", mediaPrevKind(t, "media-prev"))
	assert.Equal(t, "prev", mediaPrevKind(t, "div#media-prev"))
}

// hxTargetKind resolves a target predicate by serving one route that answers
// with the label the predicate chose, which is how each wantsX helper is
// exercised without standing up the whole page.
//
// Parameters:
//   - t: The test the request belongs to.
//   - route: Route to serve.
//   - hxTarget: HX-Target header value, empty to omit the header.
//   - hit: Body sent when the predicate matches.
//   - match: The predicate under test.
//
// Returns:
//   - body: The response body.
func hxTargetKind(
	t *testing.T,
	route, hxTarget, hit string,
	match func(fiber.Ctx) bool,
) string {
	t.Helper()

	app := fiber.New()
	app.Get(route, func(ctx fiber.Ctx) error {
		if match(ctx) {
			return ctx.SendString(hit)
		}

		return ctx.SendString("page")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, route, nil)
	req.Header.Set("Hx-Request", "true")

	if hxTarget != "" {
		req.Header.Set("Hx-Target", hxTarget)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body)
}

func mediaResultsKind(t *testing.T, hxTarget string) string {
	t.Helper()

	return hxTargetKind(t, "/media", hxTarget, "fragment", wantsMediaResults)
}

func mediaPrevKind(t *testing.T, hxTarget string) string {
	t.Helper()

	return hxTargetKind(t, "/media", hxTarget, "prev", wantsMediaPrev)
}

func mediaMoreKind(t *testing.T, hxTarget string) string {
	t.Helper()

	return hxTargetKind(t, "/media", hxTarget, "more", wantsMediaMore)
}
