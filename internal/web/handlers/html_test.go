// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

const testTVShows = "TV Shows"

func TestMediaCrumbsUsesLibraryTitle(t *testing.T) {
	t.Parallel()

	libs := []view.LibraryItem{{ID: "2", Title: testTVShows, Type: "show"}}
	crumbs := mediaCrumbs(libs, "2", "", "", "")
	require.Len(t, crumbs, 2)
	assert.Equal(t, testTVShows, crumbs[1].Title)
	assert.Equal(t, "/media?library=2", crumbs[1].URL)
}

func TestFilterClipItemsPendingIncludesProcessing(t *testing.T) {
	t.Parallel()

	items := []view.ClipItem{
		{ID: "1", Status: view.ClipStatusPending},
		{ID: "2", Status: view.ClipStatusProcessing},
		{ID: "3", Status: view.ClipStatusCompleted},
		{ID: "4", Status: view.ClipStatusFailed},
	}

	pending := filterClipItems(items, view.ClipStatusPending)
	require.Len(t, pending, 2)
	assert.Equal(t, "1", pending[0].ID)
	assert.Equal(t, "2", pending[1].ID)

	failed := filterClipItems(items, view.ClipStatusFailed)
	require.Len(t, failed, 1)
	assert.Equal(t, "4", failed[0].ID)
}

func TestFormatClipCreated(t *testing.T) {
	t.Parallel()

	assert.Empty(t, formatClipCreated(time.Time{}))
	assert.Equal(
		t,
		"Sep 3, 2026 4:32 AM",
		formatClipCreated(time.Date(2026, time.September, 3, 4, 32, 20, 0, time.UTC)),
	)
}

func TestItemCrumbsEpisodeTrail(t *testing.T) {
	t.Parallel()

	crumbs := itemCrumbs(plex.MediaItem{
		Title:            "Episode 3",
		Type:             plex.TypeEpisode,
		LibraryID:        "2",
		LibraryTitle:     testTVShows,
		ParentID:         "10",
		ParentTitle:      "Season 2",
		GrandparentID:    "9",
		GrandparentTitle: "Better Call Saul",
	}, nil)

	require.Len(t, crumbs, 5)
	assert.Equal(t, "Libraries", crumbs[0].Title)
	assert.Equal(t, testTVShows, crumbs[1].Title)
	assert.Equal(t, "Better Call Saul", crumbs[2].Title)
	assert.Equal(t, "Season 2", crumbs[3].Title)
	assert.Equal(t, "Episode 3", crumbs[4].Title)
	assert.Empty(t, crumbs[4].URL)
}

func TestItemCrumbsSeasonUsesParentShow(t *testing.T) {
	t.Parallel()

	crumbs := itemCrumbs(plex.MediaItem{
		Title:        "Season 2",
		Type:         plex.TypeSeason,
		LibraryID:    "2",
		LibraryTitle: testTVShows,
		ParentID:     "9",
		ParentTitle:  "Better Call Saul",
	}, nil)

	require.Len(t, crumbs, 4)
	assert.Equal(t, "Libraries", crumbs[0].Title)
	assert.Equal(t, testTVShows, crumbs[1].Title)
	assert.Equal(t, "Better Call Saul", crumbs[2].Title)
	assert.Equal(t, "Season 2", crumbs[3].Title)
	assert.Contains(t, crumbs[2].URL, "parent=9")
}

func TestMediaItemLocationEscapesID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/media/item/148392", mediaItemLocation("148392", url.Values{}))
	assert.Equal(
		t,
		"/media/item/a%2Fb",
		mediaItemLocation("a/b", url.Values{}),
	)
}

func TestChooserLibraries(t *testing.T) {
	t.Parallel()

	libs := []view.LibraryItem{{ID: "1", Title: "Movies", Type: "movie"}}

	assert.Equal(t, libs, chooserLibraries(libs, "", ""))
	assert.Nil(t, chooserLibraries(libs, "", "1"))
	assert.Nil(t, chooserLibraries(libs, "query", ""))
	assert.Nil(t, chooserLibraries(libs, "query", "1"))
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
		assert.Equal(t, test.want, selectedLibraryID(test.giveCurrent, test.giveQuery))
	}
}

func TestWantsMediaResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		giveTarget string
		want       string
	}{
		{giveTarget: "", want: "page"},
		{giveTarget: "main-content", want: "page"},
		{giveTarget: "media-results", want: "fragment"},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, mediaResultsKind(t, test.giveTarget))
	}
}

func mediaResultsKind(t *testing.T, hxTarget string) string {
	t.Helper()

	app := fiber.New()
	app.Get("/media", func(ctx fiber.Ctx) error {
		if wantsMediaResults(ctx) {
			return ctx.SendString("fragment")
		}

		return ctx.SendString("page")
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/media", nil)
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

func TestMediaItemError(t *testing.T) {
	t.Parallel()

	_ = t.Context()

	assert.Equal(t, "bad duration", mediaItemError(nil, "bad duration"))
	assert.Equal(t, "bad duration", mediaItemError(errNoPlexServer, "bad duration"))
	assert.Equal(t, mediaLoadFailedMsg, mediaItemError(errNoPlexServer, ""))
	assert.Empty(t, mediaItemError(nil, ""))
}

func TestClipProfileName(t *testing.T) {
	t.Parallel()

	profiles := []view.ClipProfileOption{
		{ID: "medium", Name: "Medium", IsDefault: true},
		{ID: "archive", Name: "Archive", IsDefault: false},
	}

	assert.Equal(t, "Archive", clipProfileName("archive", profiles))
	assert.Equal(t, "low", clipProfileName("low", profiles))
	assert.Empty(t, clipProfileName("", nil))
}

func TestClipFileExists(t *testing.T) {
	t.Parallel()

	assert.False(t, clipFileExists(""))
	assert.False(t, clipFileExists(filepath.Join(t.TempDir(), "missing.mp4")))

	path := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	assert.True(t, clipFileExists(path))
}

func TestAudioTrackLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"eng · dts · 7.1 · DTS:X 7.1",
		audioTrackLabel(media.AudioTrack{
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
		audioTrackLabel(media.AudioTrack{
			Index:    1,
			Codec:    "",
			Language: "",
			Title:    "",
			Channels: 0,
		}),
	)
}
