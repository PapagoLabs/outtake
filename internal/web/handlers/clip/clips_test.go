// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// clipRowTestHandler builds a handler whose queue holds the given clips.
//
// Parameters:
//   - t: The test the handler belongs to.
//   - jobs: Clips the queue should hold.
//
// Returns:
//   - handler: The handler under test.
func clipRowTestHandler(t *testing.T, jobs ...*clipdom.Job) *Handler {
	t.Helper()

	db, err := database.New(t.TempDir() + "/page_clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return &Handler{
		clipQueue:   queueForTest(t, jobs...),
		clipStorage: &blob.Storage{},
		db:          db,
		cfg:         &config.Config{MaxClipDur: 15 * time.Minute},
		sources:     stubSources(t, 2*time.Hour),
	}
}

// clipRowRequest renders a clip status row for one clip.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - path: Request path.
//
// Returns:
//   - status: Response status code.
//   - body: Response body.
func clipRowRequest(t *testing.T, handler *Handler, path string) (int, string) {
	t.Helper()

	app := fiber.New()
	app.Get("/api/clips/:id/row", handler.ClipRow)

	resp, err := app.Test(
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(body)
}

func TestClipRowShowsAProcessingBar(t *testing.T) {
	t.Parallel()

	handler := clipRowTestHandler(t, &clipdom.Job{
		ID:   "old-clip",
		Type: clipdom.TypeClip, Status: clipdom.StatusProcessing,
		Progress: 42,
	})

	status, row := clipRowRequest(t, handler, "/api/clips/"+"old-clip"+"/row")

	require.Equal(t, fiber.StatusOK, status)
	assert.Contains(t, row, "42%", "a live render shows its progress")
	assert.NotContains(t, row, "<form", "a live render offers no edit form")
}

func TestClipRowReportsAFailedClip(t *testing.T) {
	t.Parallel()

	handler := clipRowTestHandler(t, &clipdom.Job{
		ID:   "old-clip",
		Type: clipdom.TypeClip, Status: clipdom.StatusFailed,
		Progress: 37,
		Error:    "crop detected zero width",
	})

	status, row := clipRowRequest(t, handler, "/api/clips/"+"old-clip"+"/row")

	require.Equal(t, fiber.StatusOK, status)
	assert.Contains(t, row, "crop detected zero width", "the failure text reaches the card")
	assert.Contains(t, row, ">Failed<", "the status badge says the clip did not render")
	assert.NotContains(t, row, ">37%<", "a failed clip reports no live progress")
}

func TestClipRowIsAddressableAsTheCardsStatusRegion(t *testing.T) {
	t.Parallel()

	job := testClipJob("old-clip", clipdom.TypeClip)

	job.OutputPath = seedRenderedClip(t, filepath.Join(t.TempDir(), "clipdom.mp4"))

	handler := clipRowTestHandler(t, job)

	status, row := clipRowRequest(t, handler, "/api/clips/"+"old-clip"+"/row")

	require.Equal(t, fiber.StatusOK, status)
	assert.Contains(t, row, `id="clip-`+"old-clip"+`-status"`,
		"the row is addressable so the card can swap exactly this region in place")
	assert.Contains(t, row, `hx-target="#clip-`+"old-clip"+`"`,
		"the actions in the row address the card the row belongs to")
}

func TestClipRowReportsACompletedClipWhoseFileIsGone(t *testing.T) {
	t.Parallel()

	job := testClipJob("old-clip", clipdom.TypeClip)

	job.OutputPath = filepath.Join(t.TempDir(), "never-rendered.mp4")

	handler := clipRowTestHandler(t, job)

	status, row := clipRowRequest(t, handler, "/api/clips/"+"old-clip"+"/row")

	require.Equal(t, fiber.StatusOK, status)
	assert.Contains(t, row, ">Completed<",
		"the clip did finish, so its status is still completed")
	assert.Contains(t, row, "Missing File",
		"the card says the file is gone, so the user knows a re-render is needed")
	assert.NotContains(t, row, "/clips/"+"old-clip"+"/file",
		"a clip with no file offers no player pointing at it")
	assert.NotContains(t, row, "Jan 1, 2026 12:00 AM",
		"the status row is not the whole card, so the creation stamp is not repeated")
}

func TestClipRowOfAMissingClipIsNotFound(t *testing.T) {
	t.Parallel()

	handler := clipRowTestHandler(t)

	status, _ := clipRowRequest(t, handler, "/api/clips/absent/row")

	assert.Equal(t, fiber.StatusNotFound, status)
}

func TestWantsClipList(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "page", clipListKind(t, ""))
	assert.Equal(t, "page", clipListKind(t, "main-content"))
	assert.Equal(t, "fragment", clipListKind(t, "clip-list"))
	assert.Equal(t, "fragment", clipListKind(t, "div#clip-list"))
}

func clipListKind(t *testing.T, hxTarget string) string {
	t.Helper()

	return hxTargetKind(t, "/clips", hxTarget, "fragment", wantsClipList)
}

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

func TestParseClipListQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		give string
		want catalog.ClipListQuery
	}{
		{
			give: "/clips",
			want: catalog.ClipListQuery{Sort: catalog.SortCreatedDesc},
		},
		{
			give: "/clips?type=gif&q=Intro&sort=name_asc&status=completed",
			want: catalog.ClipListQuery{
				Status: clipdom.StatusCompleted,
				Type:   clipdom.TypeGIF,
				Query:  "Intro",
				Sort:   catalog.SortNameAsc,
			},
		},
		{
			give: "/clips?type=nope&sort=bogus&q=%20Clip%20",
			want: catalog.ClipListQuery{Query: "Clip", Sort: catalog.SortCreatedDesc},
		},
		{
			give: "/clips?type=screenshot&sort=updated_desc",
			want: catalog.ClipListQuery{
				Type: clipdom.TypeScreenshot,
				Sort: catalog.SortUpdatedDesc,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.give, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, parseClipListQueryFrom(t, test.give))
		})
	}
}

func parseClipListQueryFrom(t *testing.T, target string) catalog.ClipListQuery {
	t.Helper()

	app := fiber.New()

	var parsed catalog.ClipListQuery

	app.Get("/clips", func(ctx fiber.Ctx) error {
		parsed = parseClipListQuery(ctx)

		return nil
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	_, _ = io.Copy(io.Discard, resp.Body)

	return parsed
}
