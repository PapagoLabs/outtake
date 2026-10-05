// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// clipsApp mounts the clips page routes on a fresh app.
//
// Parameters:
//   - handler: The handler under test.
//
// Returns:
//   - app: The app the routes are mounted on.
func clipsApp(handler *Handler) *fiber.App {
	app := fiber.New()
	app.Get(routes.PathClips, handler.Clips)
	app.Get("/clips/:id/file", handler.ClipFile)

	return app
}

// getClips serves one clips page request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - target: Request target, including any query string.
//   - hxTarget: HX-Target header value, empty to omit the header.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func getClips(
	t *testing.T,
	handler *Handler,
	target string,
	hxTarget string,
) pageAnswer {
	t.Helper()

	return serveMethod(t, clipsApp(handler), http.MethodGet, target,
		hxTarget != "", hxTarget, "")
}

// storeClipped persists one clip for the clips page to read.
//
// Parameters:
//   - t: The test the clip belongs to.
//   - db: Database the clip is stored in.
//   - id: Identifier the clip is stored under.
//   - kind: Type of clip to store.
//   - status: Status the clip is in.
func storeClipped(
	t *testing.T,
	db *database.DB,
	id string,
	kind clipdom.Type,
	status clipdom.Status,
) {
	t.Helper()

	job := testClipJob(id, kind)

	job.Status = status

	require.NoError(t, db.SaveClip(t.Context(), job))
}

func TestClipsRendersTheStoredClips(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeClipped(t, db, "completed-clip", clipdom.TypeClip, clipdom.StatusCompleted)

	answer := getClips(t, handler, routes.PathClips, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Intro",
		"a clip that was made has to be on the page")
}

func TestClipsRendersWithNothingStored(t *testing.T) {
	t.Parallel()

	handler, _ := clipPageHandler(t)

	answer := getClips(t, handler, routes.PathClips, "")

	require.Equal(t, fiber.StatusOK, answer.status,
		"an install with no clips still gets the page, not an error")
}

func TestClipsRendersTheListFragmentForHTMX(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeClipped(t, db, "completed-clip", clipdom.TypeClip, clipdom.StatusCompleted)

	answer := getClips(t, handler, routes.PathClips, routes.TargetClipList)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Intro",
		"the list the page swaps in carries the clips on its own")
	assertBodyOmits(t, answer.body, "<html",
		"a targeted swap answers with the fragment, not a whole document")
}

func TestClipsFollowsTheCarriedFilters(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeClipped(t, db, "completed-clip", clipdom.TypeClip, clipdom.StatusCompleted)
	storeClipped(t, db, "animated", clipdom.TypeGIF, clipdom.StatusPending)

	answer := getClips(t, handler,
		routes.PathClips+"?"+url.Values{"type": {string(clipdom.TypeGIF)}}.Encode(), "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `id="clip-animated"`,
		"the requested type is in the list")
	assertBodyOmits(t, answer.body, `id="clip-`+"completed-clip"+`"`,
		"a type filter has to narrow the list the page renders")
}

func TestClipsFollowsTheCarriedStatus(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeClipped(t, db, "completed-clip", clipdom.TypeClip, clipdom.StatusCompleted)
	storeClipped(t, db, "busy", clipdom.TypeClip, clipdom.StatusProcessing)

	answer := getClips(t, handler,
		routes.PathClips+"?"+url.Values{"status": {"completed"}}.Encode(), "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `id="clip-`+"completed-clip"+`"`,
		"the requested status is in the list")
	assertBodyOmits(t, answer.body, `id="clip-busy"`,
		"a status filter has to narrow the list the page renders")
}

func TestClipsFollowsTheCarriedSearch(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeClipped(t, db, "completed-clip", clipdom.TypeClip, clipdom.StatusCompleted)

	answer := getClips(t, handler,
		routes.PathClips+"?"+url.Values{queryQ: {"Intro"}}.Encode(), "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `value="`+"Intro"+`"`,
		"the search the page was opened with is prefilled")
	assertBodyContains(t, answer.body, `id="clip-`+"completed-clip"+`"`,
		"the search matches the stored clip, so it comes back")
}

func TestClipFileRejectsAClipThatIsNotFinished(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeClipped(t, db, "busy", clipdom.TypeClip, clipdom.StatusProcessing)

	answer := getClips(t, handler, "/clips/busy/file", "")

	assert.Equal(t, fiber.StatusNotFound, answer.status,
		"a clip that is still rendering has no file to stream")
}

func TestClipFileRejectsAFinishedClipWithNoFile(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeClipped(t, db, "completed-clip", clipdom.TypeClip, clipdom.StatusCompleted)

	answer := getClips(t, handler, "/clips/"+"completed-clip"+"/file", "")

	assert.Equal(t, fiber.StatusNotFound, answer.status,
		"a finished clip whose file was deleted has nothing to stream")
}

func TestClipFileRejectsAClipNothingIsStoredUnder(t *testing.T) {
	t.Parallel()

	handler, _ := clipPageHandler(t)

	answer := getClips(t, handler, "/clips/"+"never-made"+"/file", "")

	assert.Equal(t, fiber.StatusNotFound, answer.status)
}

func TestClipFileStreamsAFinishedClip(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)

	rendered := seedRenderedClip(t, t.TempDir()+"/rendered.mp4")

	job := testClipJob("playable", clipdom.TypeClip)

	job.Status = clipdom.StatusCompleted
	job.OutputPath = rendered
	require.NoError(t, db.SaveClip(t.Context(), job))

	answer := getClips(t, handler, "/clips/playable/file", "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "rendered",
		"the bytes on disk are what the browser plays")
}

func TestParseClipListQueryReadsEveryFilter(t *testing.T) {
	t.Parallel()

	target := routes.PathClips + "?" + url.Values{
		"type":    {string(clipdom.TypeGIF)},
		queryQ:    {"  needle  "},
		querySort: {string(catalog.SortNameAsc)},
		"status":  {"completed"},
	}.Encode()

	query := clipQueryOf(t, target)

	assert.Equal(t, clipdom.TypeGIF, query.Type)
	assert.Equal(t, "needle", query.Query, "the name search is trimmed")
	assert.Equal(t, clipdom.StatusCompleted, query.Status)
	assert.Equal(t, catalog.SortNameAsc, query.Sort)
}

// clipQueryOf reads the list filters a clips page request carries.
//
// Parameters:
//   - t: The test the read belongs to.
//   - target: Request target, including any query string.
//
// Returns:
//   - query: The normalized list filters.
func clipQueryOf(t *testing.T, target string) catalog.ClipListQuery {
	t.Helper()

	var query catalog.ClipListQuery

	app := fiber.New()
	app.Get(routes.PathClips, func(ctx fiber.Ctx) error {
		query = parseClipListQuery(ctx)

		return ctx.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, target, nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	return query
}

func TestParseClipListQueryNormalizesAnUnknownSort(t *testing.T) {
	t.Parallel()

	target := routes.PathClips + "?" + url.Values{
		querySort: {"sideways"},
	}.Encode()

	assert.Equal(t, catalog.SortCreatedDesc, clipQueryOf(t, target).Sort,
		"an ordering the page does not offer falls back to the one it opens with")
}

func TestParseClipListQueryNormalizesAnEmptyForm(t *testing.T) {
	t.Parallel()

	query := clipQueryOf(t, routes.PathClips)

	assert.Empty(t, query.Type)
	assert.Empty(t, query.Query)
	assert.Empty(t, query.Status)
	assert.Equal(t, catalog.SortCreatedDesc, query.Sort)
}
