// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package deps

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/plex"
	sharedclip "github.com/PapagoLabs/outtake/internal/web/handlers/shared/clip"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	viewclip "github.com/PapagoLabs/outtake/internal/web/view/clip"
	viewmedia "github.com/PapagoLabs/outtake/internal/web/view/media"
)

const (
	testTVShows    = "TV Shows"
	testShow       = "Show"
	testSeason     = "Season 3"
	testEpisode    = "Episode 5"
	testShowURL    = "/media?library=2&parent=9&title=Show"
	testSeasonURL  = "/media?library=2&parent=10&title=Season+3&up=9&upTitle=Show"
	testEpisodeURL = "/media/item/42"
	testMovie      = "Movie"
)

func TestMediaCrumbsUsesLibraryTitle(t *testing.T) {
	t.Parallel()

	libs := []viewmedia.LibraryItem{{ID: "2", Title: testTVShows, Type: "show"}}
	crumbs := MediaCrumbs(libs, "2", "", "", "")
	require.Len(t, crumbs, 2)
	assert.Equal(t, testTVShows, crumbs[1].Title)
	assert.Equal(t, "/media?library=2", crumbs[1].URL)
}

func TestFormatClipCreated(t *testing.T) {
	t.Parallel()

	assert.Empty(t, FormatClipCreated(time.Time{}))
	assert.Equal(
		t,
		"Sep 3, 2026 4:32 AM",
		FormatClipCreated(time.Date(2026, time.September, 3, 4, 32, 20, 0, time.UTC)),
	)
}

func TestItemCrumbsEpisodeTrail(t *testing.T) {
	t.Parallel()

	crumbs := ItemCrumbs(plex.MediaItem{
		Title:            "Episode 3",
		Type:             plex.TypeEpisode,
		LibraryID:        "2",
		LibraryTitle:     testTVShows,
		ParentID:         "10",
		ParentTitle:      "Season 2",
		GrandparentID:    "9",
		GrandparentTitle: testShow,
	}, nil)

	require.Len(t, crumbs, 5)
	assert.Equal(t, "Libraries", crumbs[0].Title)
	assert.Equal(t, testTVShows, crumbs[1].Title)
	assert.Equal(t, testShow, crumbs[2].Title)
	assert.Equal(t, "Season 2", crumbs[3].Title)
	assert.Equal(t, "Episode 3", crumbs[4].Title)
	assert.Empty(t, crumbs[4].URL)
}

func TestItemCrumbsSeasonUsesParentShow(t *testing.T) {
	t.Parallel()

	crumbs := ItemCrumbs(plex.MediaItem{
		Title:        "Season 2",
		Type:         plex.TypeSeason,
		LibraryID:    "2",
		LibraryTitle: testTVShows,
		ParentID:     "9",
		ParentTitle:  testShow,
	}, nil)

	require.Len(t, crumbs, 4)
	assert.Equal(t, "Libraries", crumbs[0].Title)
	assert.Equal(t, testTVShows, crumbs[1].Title)
	assert.Equal(t, testShow, crumbs[2].Title)
	assert.Equal(t, "Season 2", crumbs[3].Title)
	assert.Contains(t, crumbs[2].URL, "parent=9")
}

func TestSessionTitleParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		give     plex.MediaItem
		want     []viewmedia.Crumb
		wantYear int
	}{
		{
			name: "episode with ids",
			give: plex.MediaItem{
				ID:               "42",
				Title:            testEpisode,
				Type:             plex.TypeEpisode,
				LibraryID:        "2",
				ParentID:         "10",
				ParentTitle:      testSeason,
				GrandparentID:    "9",
				GrandparentTitle: testShow,
			},
			want: []viewmedia.Crumb{
				{Title: testShow, URL: testShowURL},
				{Title: testSeason, URL: testSeasonURL},
				{Title: testEpisode, URL: testEpisodeURL},
			},
		},
		{
			name: "episode missing library",
			give: plex.MediaItem{
				ID:               "42",
				Title:            testEpisode,
				Type:             plex.TypeEpisode,
				ParentID:         "10",
				ParentTitle:      testSeason,
				GrandparentID:    "9",
				GrandparentTitle: testShow,
			},
			want: []viewmedia.Crumb{
				{Title: testShow},
				{Title: testSeason},
				{Title: testEpisode, URL: testEpisodeURL},
			},
		},
		{
			name: "episode missing parent title uses index",
			give: plex.MediaItem{
				ID:               "42",
				Title:            testEpisode,
				Type:             plex.TypeEpisode,
				LibraryID:        "2",
				ParentID:         "10",
				ParentIndex:      3,
				GrandparentID:    "9",
				GrandparentTitle: testShow,
			},
			want: []viewmedia.Crumb{
				{Title: testShow, URL: testShowURL},
				{Title: testSeason, URL: testSeasonURL},
				{Title: testEpisode, URL: testEpisodeURL},
			},
		},
		{
			name: "episode skips season without title or index",
			give: plex.MediaItem{
				ID:               "42",
				Title:            testEpisode,
				Type:             plex.TypeEpisode,
				LibraryID:        "2",
				ParentID:         "10",
				GrandparentID:    "9",
				GrandparentTitle: testShow,
			},
			want: []viewmedia.Crumb{
				{Title: testShow, URL: testShowURL},
				{Title: testEpisode, URL: testEpisodeURL},
			},
		},
		{
			name: "movie with year",
			give: plex.MediaItem{
				ID:    "100",
				Title: testMovie,
				Year:  1995,
			},
			want:     []viewmedia.Crumb{{Title: testMovie, URL: "/media/item/100"}},
			wantYear: 1995,
		},
		{
			name: "movie without year",
			give: plex.MediaItem{
				ID:    "101",
				Title: testMovie,
			},
			want: []viewmedia.Crumb{{Title: testMovie, URL: "/media/item/101"}},
		},
		{
			name: "non-episode without id",
			give: plex.MediaItem{Title: "Concert", Type: "clip"},
			want: []viewmedia.Crumb{{Title: "Concert"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, year := SessionTitleParts(test.give)
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.wantYear, year)
		})
	}
}

func TestMediaItemLocationEscapesID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/media/item/148392", MediaItemLocation("148392", url.Values{}))
	assert.Equal(
		t,
		"/media/item/a%2Fb",
		MediaItemLocation("a/b", url.Values{}),
	)
}

func TestChooserLibraries(t *testing.T) {
	t.Parallel()

	libs := []viewmedia.LibraryItem{{ID: "1", Title: "Movies", Type: sharedclip.DefaultMediaType}}

	assert.Equal(t, libs, ChooserLibraries(libs, "", ""))
	assert.Nil(t, ChooserLibraries(libs, "", "1"))
	assert.Nil(t, ChooserLibraries(libs, "query", ""))
	assert.Nil(t, ChooserLibraries(libs, "query", "1"))
}

func TestSelectedLibraryID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		giveCurrent string
		giveQuery   string
		want        string
	}{
		{giveCurrent: "", giveQuery: "7", want: "7"},
		{giveCurrent: "http://localhost:8080/media?library=3", giveQuery: "", want: "3"},
		{giveCurrent: "http://localhost:8080/clips", giveQuery: "", want: ""},
		{giveCurrent: "://bad", giveQuery: "", want: ""},
		{giveCurrent: "http://localhost:8080/media?library=9", giveQuery: "1", want: "1"},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, SelectedLibraryID(test.giveCurrent, test.giveQuery))
	}
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
		assert.Equal(t, test.want, mediaResultsKind(t, test.giveTarget))
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

func TestWantsClipList(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "page", clipListKind(t, ""))
	assert.Equal(t, "page", clipListKind(t, "main-content"))
	assert.Equal(t, "fragment", clipListKind(t, "clip-list"))
	assert.Equal(t, "fragment", clipListKind(t, "div#clip-list"))
}

// hxTargetKind returns the hx target kind.
//
// Parameters:
//   - t: T.
//   - route: Route.
//   - hxTarget: Hx target.
//   - hit: Hit.
//   - match: Match.
//
// Returns:
//   - value: The hx target kind.
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

// mediaResultsKind returns the media results kind.
//
// Parameters:
//   - t: T.
//   - hxTarget: Hx target.
//
// Returns:
//   - value: The media results kind.
func mediaResultsKind(t *testing.T, hxTarget string) string {
	t.Helper()

	return hxTargetKind(t, "/media", hxTarget, "fragment", WantsMediaResults)
}

// mediaPrevKind returns the media prev kind.
//
// Parameters:
//   - t: T.
//   - hxTarget: Hx target.
//
// Returns:
//   - value: The media prev kind.
func mediaPrevKind(t *testing.T, hxTarget string) string {
	t.Helper()

	return hxTargetKind(t, "/media", hxTarget, "prev", WantsMediaPrev)
}

// mediaMoreKind returns the media more kind.
//
// Parameters:
//   - t: T.
//   - hxTarget: Hx target.
//
// Returns:
//   - value: The media more kind.
func mediaMoreKind(t *testing.T, hxTarget string) string {
	t.Helper()

	return hxTargetKind(t, "/media", hxTarget, "more", WantsMediaMore)
}

// clipListKind returns the clip list kind.
//
// Parameters:
//   - t: T.
//   - hxTarget: Hx target.
//
// Returns:
//   - value: The clip list kind.
func clipListKind(t *testing.T, hxTarget string) string {
	t.Helper()

	return hxTargetKind(t, "/clips", hxTarget, "fragment", WantsClipList)
}

func TestMediaItemError(t *testing.T) {
	t.Parallel()

	_ = t.Context()

	assert.Equal(t, "bad duration", respond.MediaItemError(nil, "bad duration"))
	assert.Equal(t, "bad duration", respond.MediaItemError(errNoPlexServer, "bad duration"))
	assert.Equal(t, respond.MediaLoadFailedMsg, respond.MediaItemError(errNoPlexServer, ""))
	assert.Empty(t, respond.MediaItemError(nil, ""))
}

func TestClipProfileName(t *testing.T) {
	t.Parallel()

	profiles := []viewclip.ClipProfileOption{
		{ID: "medium", Name: "Medium", IsDefault: true},
		{ID: "archive", Name: "Archive", IsDefault: false},
	}

	assert.Equal(t, "Archive", ClipProfileName("archive", profiles))
	assert.Equal(t, "low", ClipProfileName("low", profiles))
	assert.Empty(t, ClipProfileName("", nil))
}

func TestClipFileExists(t *testing.T) {
	t.Parallel()

	assert.False(t, ClipFileExists(""))
	assert.False(t, ClipFileExists(filepath.Join(t.TempDir(), "missing.mp4")))

	path := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	assert.True(t, ClipFileExists(path))
}

func TestAudioTrackLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"eng · dts · 7.1 · DTS:X 7.1",
		AudioTrackLabel(media.AudioTrack{
			Index:    0,
			Codec:    "dts",
			Language: "eng",
			Title:    "DTS:X 7.1",
			Channels: 8,
		}),
	)
	assert.Equal(
		t,
		"Track 2",
		AudioTrackLabel(media.AudioTrack{
			Index:    1,
			Codec:    "",
			Language: "",
			Title:    "",
			Channels: 0,
		}),
	)
}

// closeBody closes resp.Body and ignores the error.
//
// Parameters:
//   - t: T.
//   - resp: Resp.
func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}
