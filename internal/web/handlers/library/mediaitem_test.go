// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/handlers/library/mocks"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/respond/respondtest"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// countOccurrences counts how many times a substring appears in the text.
//
// Parameters:
//   - text: Text to search.
//   - want: Substring to count.
//
// Returns:
//   - count: How many times want appears in text.
func countOccurrences(text, want string) int {
	return strings.Count(text, want)
}

// clipsForMediaOf reads the clip cards the media item page would render for one
// media id, which needs a live request context.
//
// Parameters:
//   - t: The test the read belongs to.
//   - handler: The handler under test.
//   - source: Probed source information to stamp the cards with.
//
// Returns:
//   - clips: The clip cards the page would render.
func clipsForMediaOf(
	t *testing.T,
	handler *Handler,
	source library.SourceInfo,
) []view.ClipItem {
	t.Helper()

	var clips []view.ClipItem

	app := fiber.New()
	app.Get(routes.PathItemPrefix+":"+routes.ParamID, func(ctx fiber.Ctx) error {
		clips = handler.clipsForMedia(ctx, "42", source, normalizedQuery())

		return ctx.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, routes.PathItemPrefix+"42", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	return clips
}

// normalizedQuery builds the empty normalized clip list query the pages pass in
// when they want every clip in whatever order storage keeps them.
//
// Returns:
//   - query: The normalized query.
func normalizedQuery() catalog.ClipListQuery {
	return catalog.Normalize(catalog.ClipListQuery{})
}

// mediaItemApp mounts the media item routes on a fresh app.
//
// Parameters:
//   - handler: The handler under test.
//
// Returns:
//   - app: The app the routes are mounted on.
func mediaItemApp(handler *Handler) *fiber.App {
	app := fiber.New()
	app.Get(routes.PathItemPrefix+":"+routes.ParamID, handler.MediaItem)
	app.Get(routes.PathItemPrefix+":"+routes.ParamID+"/clips", handler.MediaItemClips)
	app.Get("/media/new", handler.NewClip)
	app.Get(routes.PathItemPrefix, handler.NewClip)

	return app
}

// clipJob builds a clip the media item page will list.
//
// Parameters:
//   - id: Identifier the clip is stored under.
//   - status: Status the clip is in.
//
// Returns:
//   - job: The clip.
func clipJob(id string, status clip.Status) *clip.Job {
	job := testClipJob(id, clip.TypeClip)

	job.Status = status

	return job
}

// storeClip persists a clip for the media item page to read, which it does
// through storage rather than the queue.
//
// Parameters:
//   - t: The test the clip belongs to.
//   - db: Database the clip is stored in.
//   - job: The clip to store.
func storeClip(t *testing.T, db *database.DB, job *clip.Job) {
	t.Helper()

	job.MediaID = "42"
	require.NoError(t, db.SaveClip(t.Context(), job))
}

// getItem serves one media item page request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - target: Request target, including any query string.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func getItem(t *testing.T, handler *Handler, target string) pageAnswer {
	t.Helper()

	return serve(t, mediaItemApp(handler), target, false)
}

func TestMediaItemRendersTheResolvedItem(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/library/metadata/42":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
		{"ratingKey":"42","title":"Test Movie","type":"movie","duration":7200000,
		 "year":1999,"librarySectionID":"1",
		 "Media":[{"Part":[{"file":"/movies/test.mkv"}]}]}
	]}}`))
		default:
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"},
		{"key":"2","title":"TV Shows","type":"show"}
	]}}`))
		}
	})

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Client().Return(stub.client, stub.server, true)

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().
		DescribePath(mock.Anything, "/movies/test.mkv").
		Return(sourceInfo(90 * time.Minute))

	handler, _ := pageHandler(t, auth, sources)

	answer := getItem(t, handler, routes.PathItemPrefix+"42")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "<h2",
		"the item page is what the server reported, not the id fallback")
	assertBodyContains(t, answer.body, "Test Movie (1999)",
		"the title Plex reported is the title the page shows")
	assertBodyContains(t, answer.body, `data-library="1"`,
		"the item's library scopes the media nav and the clip form")
	assertBodyContains(t, answer.body, `>Movies</a>`,
		"the breadcrumb names the library the item lives in")
	assertBodyContains(t, answer.body, `data-media-dur="7200.000"`,
		"the length Plex reported reaches the export form")
}

func TestMediaItemFallsBackToTheIDWhenPlexCannotBeReached(t *testing.T) {
	t.Parallel()

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().Describe(mock.Anything, "42").Return(sourceInfo(time.Hour))

	handler, _ := pageHandler(t, offlineAuth(t), sources)

	answer := getItem(t, handler, routes.PathItemPrefix+"42")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, ">42<",
		"with no metadata the id is the only title the page has")
	assertBodyContains(t, answer.body, html.EscapeString(mediaLoadFailedMsg),
		"the user is told the item could not be read, and that a clip may still work")
	assertBodyOmits(t, answer.body, "Test Movie",
		"nothing was read, so there is no Plex title to show")
	assertBodyContains(t, answer.body, `name="mediaId" value="42"`,
		"the export form still targets the id the browser asked about")
}

func TestMediaItemCarriesTheExportWindow(t *testing.T) {
	t.Parallel()

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().Describe(mock.Anything, "42").Return(library.SourceInfo{})

	handler, _ := pageHandler(t, offlineAuth(t), sources)

	answer := getItem(t, handler,
		routes.PathItemPrefix+"42?"+routes.QueryStart+"=10&"+routes.QueryEnd+"=42")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `id="startTime" name="startTime" type="text" `+
		`inputmode="decimal" placeholder="00:00:00.000" value="00:00:10.000"`,
		"the start mark the page was opened with is prefilled")
	assertBodyContains(t, answer.body, `name="endTime" type="text" `+
		`inputmode="decimal" placeholder="00:00:00.000" value="00:00:42.000"`,
		"the end mark the page was opened with is prefilled")
}

// TestMediaItemPrefersAFlashOverALoadFailure covers a form failure carried
// to a media page whose metadata cannot be loaded: the layout's banner shows
// the failure, and the load notice stays out of the way.
func TestMediaItemPrefersAFlashOverALoadFailure(t *testing.T) {
	t.Parallel()

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().Describe(mock.Anything, "42").Return(library.SourceInfo{})

	handler, _ := pageHandler(t, offlineAuth(t), sources)

	app := fiber.New()
	respondtest.Sessions(t, app)
	app.Post("/fail", func(ctx fiber.Ctx) error {
		respond.SetFlash(ctx, view.NewNotice("Choose a Plex server under Servers first"))

		return ctx.SendStatus(fiber.StatusNoContent)
	})
	app.Get(routes.PathItemPrefix+":"+routes.ParamID, handler.MediaItem)

	failed, err := app.Test(
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/fail", nil),
	)
	require.NoError(t, err)
	require.NoError(t, failed.Body.Close())

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		routes.PathItemPrefix+"42",
		nil,
	)
	for _, cookie := range failed.Cookies() {
		req.AddCookie(cookie)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
	assertBodyContains(t, string(body), "Choose a Plex server under Servers first",
		"the failure the post left behind is what the page shows")
	assertBodyOmits(t, string(body), html.EscapeString(mediaLoadFailedMsg),
		"a form failure the user caused outranks the load notice")
}

// TestMediaItemErrorFallsBackToTheLoadNotice covers the notice on its own:
// a failed load shows it, and an item that loaded has nothing to say.
func TestMediaItemErrorFallsBackToTheLoadNotice(t *testing.T) {
	t.Parallel()

	var failed, loaded string

	app := fiber.New()
	respondtest.Sessions(t, app)
	app.Get("/x", func(ctx fiber.Ctx) error {
		failed = mediaItemError(ctx, assert.AnError)
		loaded = mediaItemError(ctx, nil)

		return nil
	})

	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, mediaLoadFailedMsg, failed)
	assert.Empty(t, loaded, "an item that loaded and left no flash has nothing to say")
}

func TestLoadMediaItemReportsNoServerBound(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	app := fiber.New()

	var (
		gotItem plex.MediaItem
		gotErr  error
	)

	app.Get(routes.PathItemPrefix+":"+routes.ParamID, func(ctx fiber.Ctx) error {
		gotItem, gotErr = handler.loadMediaItem(ctx, "42")

		return ctx.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, routes.PathItemPrefix+"42", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	require.ErrorIs(t, gotErr, library.ErrNoServer)
	assert.Equal(t, plex.MediaItem{}, gotItem)
}

func TestLoadMediaItemReportsAFailedLookup(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Client().Return(stub.client, stub.server, true)

	handler, _ := pageHandler(t, auth, silentSources(t))

	app := fiber.New()

	var (
		gotItem plex.MediaItem
		gotErr  error
	)

	app.Get(routes.PathItemPrefix+":"+routes.ParamID, func(ctx fiber.Ctx) error {
		gotItem, gotErr = handler.loadMediaItem(ctx, "42")

		return ctx.SendStatus(fiber.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, routes.PathItemPrefix+"42", nil,
	))
	require.NoError(t, err)

	defer closeBody(t, resp)

	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "load media item",
		"the failure names the step that could not be done")
	assert.Equal(t, plex.MediaItem{}, gotItem)
}

func TestClipsForMediaReadsTheProfilesAndTheSource(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	clips := clipsForMediaOf(t, handler, sourceInfo(time.Hour))

	assert.Empty(t, clips, "no clip is stored for this media yet")
}

func TestClipsForMediaStampsEachCardFromTheSource(t *testing.T) {
	t.Parallel()

	handler, db := pageHandler(t, offlineAuth(t), silentSources(t))
	storeClip(t, db, clipJob("old-clip", clip.StatusProcessing))

	clips := clipsForMediaOf(t, handler, sourceInfo(4*time.Hour))

	require.Len(t, clips, 1)
	assert.InDelta(t, (4 * time.Hour).Seconds(), clips[0].MediaDuration.Seconds(), 0.001,
		"the card knows how long the source runs")
	assert.True(t, clips[0].SourceHDR, "the card offers to keep the probed HDR transfer")
	assert.Len(t, clips[0].AudioTracks, 2,
		"the card offers every probed audio track")
}

// TestMediaItemAsksPlexForTheItemOnce covers the media page reading the item's
// metadata a single time: the file it probes comes from the metadata it
// already holds.
func TestMediaItemAsksPlexForTheItemOnce(t *testing.T) {
	t.Parallel()

	var lookups atomic.Int32

	stub := startPMS(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/metadata/42" {
			lookups.Add(1)

			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
		{"ratingKey":"42","title":"Test Movie","type":"movie","duration":7200000,
		 "librarySectionID":"1","Media":[{"Part":[{"file":"/plex/movies/test.mkv"}]}]}
	]}}`))

			return
		}

		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[]}}`))
	})

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Client().Return(stub.client, stub.server, true)

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().
		DescribePath(mock.Anything, "/media/movies/test.mkv").
		Return(sourceInfo(time.Hour)).
		Once()

	handler, _ := pageHandler(t, auth, sources)

	handler.cfg.PlexMediaRoot = "/plex"
	handler.cfg.LocalMediaRoot = "/media"

	answer := getItem(t, handler, routes.PathItemPrefix+"42")

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Equal(t, int32(1), lookups.Load(),
		"the page reads the item's metadata once and probes the file it names")
}

// TestMediaItemProbesNothingForAnItemWithoutAFile covers an item Plex reports
// with no media part, such as a show: there is no file to describe, so the
// page neither probes nor asks Plex again.
func TestMediaItemProbesNothingForAnItemWithoutAFile(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/metadata/42" {
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
		{"ratingKey":"42","title":"Test Show","type":"show","librarySectionID":"2"}
	]}}`))

			return
		}

		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[]}}`))
	})

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Client().Return(stub.client, stub.server, true)

	handler, _ := pageHandler(t, auth, mocks.NewMockMediaDescriber(t))

	answer := getItem(t, handler, routes.PathItemPrefix+"42")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Test Show",
		"the page still renders what Plex reported")
}

func TestMediaItemClipsFiltersThroughTheListQuery(t *testing.T) {
	t.Parallel()

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().Describe(mock.Anything, "42").Return(library.SourceInfo{})

	handler, db := pageHandler(t, offlineAuth(t), sources)
	storeClip(t, db, clipJob("done", clip.StatusCompleted))
	storeClip(t, db, clipJob("busy", clip.StatusProcessing))

	answer := serve(
		t,
		mediaItemApp(handler),
		routes.PathItemPrefix+"42/clips?status=completed",
		true,
	)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `id="clip-done"`,
		"the requested status is in the fragment")
	assertBodyOmits(t, answer.body, `id="clip-busy"`,
		"a status filter has to narrow the list the page swaps in")
}

func TestMediaItemClipsRendersAGIFCard(t *testing.T) {
	t.Parallel()

	gif := testClipJob("animated", clip.TypeGIF)

	gif.Status = clip.StatusProcessing

	handler, db := pageHandler(t, offlineAuth(t), silentSources(t))
	storeClip(t, db, gif)

	answer := serve(t, mediaItemApp(handler), routes.PathItemPrefix+"42/clips", true)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `data-export-for="gif"`,
		"a GIF card offers the width and frame rate a video clip has no use for")
}

func TestMediaItemClipsRendersTheListFragment(t *testing.T) {
	t.Parallel()

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().
		Describe(mock.Anything, "42").
		Return(sourceInfo(2 * time.Hour))

	handler, db := pageHandler(t, offlineAuth(t), sources)
	storeClip(t, db, clipJob("old-clip", clip.StatusProcessing))

	answer := serve(t, mediaItemApp(handler), routes.PathItemPrefix+"42/clips", true)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `id="clip-`+"old-clip"+`"`,
		"the fragment is the clip cards the page swaps in")
	assertBodyContains(t, answer.body, `data-media-dur="7200.000"`,
		"the card is stamped with the probed source length")
}

func TestMediaItemNamesTheExportFormFromTheMediaTitle(t *testing.T) {
	t.Parallel()

	stub := startPMS(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/metadata/42" {
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
		{"ratingKey":"42","title":"Test Movie","type":"movie","duration":7200000,
		 "year":1999,"librarySectionID":"1",
		 "Media":[{"Part":[{"file":"/movies/test.mkv"}]}]}
	]}}`))

			return
		}

		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
		{"key":"1","title":"Movies","type":"movie"},
		{"key":"2","title":"TV Shows","type":"show"}
	]}}`))
	})

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().Client().Return(stub.client, stub.server, true)

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().DescribePath(mock.Anything, "/movies/test.mkv").Return(library.SourceInfo{})

	handler, _ := pageHandler(t, auth, sources)

	answer := getItem(t, handler, routes.PathItemPrefix+"42")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `name="name"`,
		"the export form is rendered")
	assertBodyContains(t, answer.body, `value="Test Movie (1999)" class="`,
		"the export form is prefilled from the media title, which is only "+
			"resolved before the form is built")
}

func TestMediaItemStampsEveryCardWithTheSourceInformation(t *testing.T) {
	t.Parallel()

	sources := mocks.NewMockMediaDescriber(t)
	sources.EXPECT().
		Describe(mock.Anything, "42").
		Return(sourceInfo(3 * time.Hour))

	handler, db := pageHandler(t, offlineAuth(t), sources)
	storeClip(t, db, clipJob("first", clip.StatusProcessing))
	storeClip(t, db, clipJob("second", clip.StatusProcessing))

	answer := serve(t, mediaItemApp(handler), routes.PathItemPrefix+"42/clips", true)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Equal(t, 2, countOccurrences(answer.body, `name="audioIndex"`),
		"each card offers the probed audio tracks")
	assert.Zero(t, countOccurrences(answer.body, `name="preserveHdr"`),
		"Keep HDR is the profile's, so no card offers it")
	assert.Equal(t, 2, countOccurrences(answer.body, `name="endTime"`),
		"each card is stamped with its own end mark")
}

func TestNewClipCarriesAStartMark(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := serve(t, mediaItemApp(handler),
		"/media/new?mediaId=42&"+routes.QueryStart+"=12.5",
		false)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathItemPrefix+"42?"+routes.QueryStart+"=12.5",
		answer.header.Get(fiber.HeaderLocation),
		"the mark the user set has to survive the hop to the editor")
}

func TestNewClipCarriesNoMarkWhenNoneWasSet(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := serve(t, mediaItemApp(handler),
		"/media/new?mediaId=42",
		false)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathItemPrefix+"42", answer.header.Get(fiber.HeaderLocation),
		"an empty mark is not worth carrying, so the item page is asked for plainly")
}

func TestNewClipSendsAnEmptyMediaIDToTheBrowser(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := serve(t, mediaItemApp(handler), "/media/new", false)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathMedia, answer.header.Get(fiber.HeaderLocation),
		"with no media id there is no item page to open")
}
