// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
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
	app.Get("/clips/:id/sdr", handler.ClipSDRFile)

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

// storedOnlyRemotely stores a finished clip whose file the configured storage
// holds but this host does not, as on S3 after a restart.
//
// Parameters:
//   - t: The test the clip belongs to.
//
// Returns:
//   - handler: A clips handler over a mocked store.
//   - store: The mocked store.
//   - output: Where the clip's file lands once fetched.
func storedOnlyRemotely(t *testing.T) (*Handler, *storagemocks.MockBlob, string) {
	t.Helper()

	handler, db := clipPageHandler(t)

	store := storagemocks.NewMockBlob(t)

	handler.clipStorage = store

	output := filepath.Join(t.TempDir(), "remote.mp4")

	job := testClipJob("remote", clipdom.TypeClip)

	job.OutputPath = output
	require.NoError(t, db.SaveClip(t.Context(), job))

	return handler, store, output
}

// TestClipFileFetchesAFileOnlyTheStoreHolds covers inline playback on S3: the
// file is fetched to local disk and streamed.
func TestClipFileFetchesAFileOnlyTheStoreHolds(t *testing.T) {
	t.Parallel()

	handler, store, output := storedOnlyRemotely(t)

	store.EXPECT().Ensure(mock.Anything, output).RunAndReturn(func(context.Context, string) error {
		return os.WriteFile(output, []byte("video"), 0o600)
	}).Once()

	answer := getClips(t, handler, "/clips/remote/file", "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Equal(t, "video", answer.body)
}

// TestClipFileRejectsAFileTheStoreCannotFetch covers a file gone from storage.
func TestClipFileRejectsAFileTheStoreCannotFetch(t *testing.T) {
	t.Parallel()

	handler, store, output := storedOnlyRemotely(t)

	store.EXPECT().Ensure(mock.Anything, output).Return(fs.ErrNotExist).Once()

	assert.Equal(t, fiber.StatusNotFound, getClips(t, handler, "/clips/remote/file", "").status)
}

// TestClipsAsksTheStoreWhetherAFileExists covers the card badge on S3: a file
// the store holds is not reported missing, and nothing is downloaded to say so.
func TestClipsAsksTheStoreWhetherAFileExists(t *testing.T) {
	t.Parallel()

	handler, store, output := storedOnlyRemotely(t)

	store.EXPECT().Exists(mock.Anything, output).Return(true)

	answer := getClips(t, handler, routes.PathClips, "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.NotContains(t, answer.body, "Missing file")
}

// TestClipsOffersNoHDRChoice covers the clips page: Keep HDR is the profile's,
// so no card offers it, whether its source is SDR or HDR.
func TestClipsOffersNoHDRChoice(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeClipped(t, db, "hdr-clip", clipdom.TypeClip, clipdom.StatusCompleted)

	answer := getClips(t, handler, routes.PathClips, "")
	require.Equal(t, fiber.StatusOK, answer.status)
	assert.NotContains(t, answer.body, `name="preserveHdr"`)

	handler.sources = hdrSources(t)

	answer = getClips(t, handler, routes.PathClips, "")
	require.Equal(t, fiber.StatusOK, answer.status)
	assert.NotContains(t, answer.body, `name="preserveHdr"`)
	assert.Contains(t, answer.body, `name="cropBlackBars"`, "trim black bars is still offered")
}

// storeHDRClip persists a finished video clip that keeps HDR, with its own
// file and, when asked, its SDR version beside it.
//
// Parameters:
//   - t: The test the clip belongs to.
//   - db: Database the clip is stored in.
//   - id: Identifier the clip is stored under.
//   - withSDR: Whether the SDR version is stored too.
func storeHDRClip(t *testing.T, db *database.DB, id string, withSDR bool) {
	t.Helper()

	job := testClipJob(id, clipdom.TypeClip)

	job.Status = clipdom.StatusCompleted
	job.PreserveHDR = true
	job.OutputPath = seedRenderedClip(t, filepath.Join(t.TempDir(), id+".mp4"))

	if withSDR {
		require.NoError(t, os.WriteFile(job.SDRPath(), []byte("sdr version"), 0o600))
	}

	require.NoError(t, db.SaveClip(t.Context(), job))
}

// TestClipSDRFileStreamsTheSDRVersion covers the route a card plays an HDR
// clip's SDR version from.
func TestClipSDRFileStreamsTheSDRVersion(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeHDRClip(t, db, "hdr", true)

	answer := getClips(t, handler, "/clips/hdr/sdr", "")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "sdr version", "the SDR version's bytes are sent")
}

// TestClipSDRFileRefusesAClipWithNone covers a clip with no SDR version: one
// that converts to SDR, and an HDR clip rendered before SDR versions existed.
func TestClipSDRFileRefusesAClipWithNone(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)
	storeHDRClip(t, db, "older", false)

	converted := testClipJob("converted", clipdom.TypeClip)

	converted.Status = clipdom.StatusCompleted
	converted.OutputPath = seedRenderedClip(t, filepath.Join(t.TempDir(), "converted.mp4"))
	require.NoError(t, db.SaveClip(t.Context(), converted))

	assert.Equal(t, fiber.StatusNotFound, getClips(t, handler, "/clips/older/sdr", "").status)
	assert.Equal(t, fiber.StatusNotFound, getClips(t, handler, "/clips/converted/sdr", "").status)
}

// TestClipsPlaysTheSDRVersionFirst covers the card of an HDR clip: it starts
// on the SDR version and names the HDR file for the page script, and a clip
// with no SDR version is marked so the script can say so.
func TestClipsPlaysTheSDRVersionFirst(t *testing.T) {
	t.Parallel()

	handler, db := clipPageHandler(t)

	handler.sources = hdrSources(t)

	storeHDRClip(t, db, "with-sdr", true)
	storeHDRClip(t, db, "without-sdr", false)

	answer := getClips(t, handler, routes.PathClips, "")
	require.Equal(t, fiber.StatusOK, answer.status)

	assertBodyContains(t, answer.body, `src="/clips/with-sdr/sdr?v=`, "the SDR version plays first")
	assertBodyContains(t, answer.body, `data-hdr-src="/clips/with-sdr/file?v=`,
		"and the HDR file is named for screens that show it")
	assertBodyContains(t, answer.body, `src="/clips/without-sdr/file?v=`,
		"a clip with no SDR version plays its own file")
	assertBodyContains(t, answer.body, "data-sdr-missing", "and is marked as lacking one")
}
