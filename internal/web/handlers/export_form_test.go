// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
)

// exportFormFromTarget runs a redirect location back through the query reader,
// which is the full hand-off a preview round trip performs.
func exportFormFromTarget(t *testing.T, location string, defaultCrop bool) exportFormState {
	t.Helper()

	return exportFormFromTargetWithTitle(t, location, defaultCrop, testMovie)
}

// exportFormFromTargetWithTitle is exportFormFromTarget with an explicit media
// title, for the cases where the title fallback is under test.
func exportFormFromTargetWithTitle(
	t *testing.T,
	location string,
	defaultCrop bool,
	title string,
) exportFormState {
	t.Helper()

	app := fiber.New()

	var got exportFormState

	app.Get("/media/item/:id", func(ctx fiber.Ctx) error {
		got = exportFormFromQuery(ctx, defaultCrop, title)

		return nil
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, location, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	return got
}

// TestExportFormSurvivesPreviewRedirect is the regression guard for this
// branch. Every field below is discarded by the old redirect, so the form
// resets to its defaults partway through an edit.
func TestExportFormSurvivesPreviewRedirect(t *testing.T) {
	t.Parallel()

	req := api.ClipRequest{
		MediaID:       "42",
		ClipType:      clipTypeGIF,
		Name:          "A name with spaces & an ampersand",
		Quality:       "profile-high",
		AudioIndex:    3,
		Width:         1280,
		FPS:           24,
		CropBlackBars: true,
		WebSafeColor:  new(true),
		StartTime:     10,
		Duration:      5,
	}

	location := previewRedirectURL(
		req.MediaID,
		"preview-1",
		req.StartTime,
		req.StartTime+req.Duration,
		req.WebSafeColor,
		exportFormFromRequest(req),
	)

	got := exportFormFromTarget(t, location, false)

	assert.Equal(t, clipTypeGIF, got.Type)
	assert.Equal(t, "A name with spaces & an ampersand", got.Name)
	assert.Equal(t, "profile-high", got.Quality)
	assert.Equal(t, 3, got.AudioIndex)
	assert.Equal(t, 1280, got.Width)
	assert.Equal(t, 24, got.FPS)
	assert.True(t, got.CropBlackBars)
}

// TestExportFormCarriesExplicitCropOptOut covers the other direction. The
// toggle has to be written even when false, or the form cannot tell "the user
// turned it off" from "this is a first visit using the config default".
func TestExportFormCarriesExplicitCropOptOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		defaultCrop bool
		want        bool
	}{
		{name: "explicit off beats an on config default", defaultCrop: true, want: false},
		{name: "explicit on beats an off config default", defaultCrop: false, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			location := previewRedirectURL(
				"42",
				"preview-1",
				0,
				5,
				nil,
				exportFormState{CropBlackBars: !test.defaultCrop},
			)

			assert.Equal(
				t,
				test.want,
				exportFormFromTarget(t, location, test.defaultCrop).CropBlackBars,
			)
		})
	}
}

// TestExportFormAbsentCropFallsBackToConfig covers a first visit, which arrives
// with no query at all and should show the configured default. A preview
// redirect always carries the toggle, because the form always submits it.
func TestExportFormAbsentCropFallsBackToConfig(t *testing.T) {
	t.Parallel()

	assert.True(t, exportFormFromTarget(t, "/media/item/42", true).CropBlackBars)
	assert.False(t, exportFormFromTarget(t, "/media/item/42", false).CropBlackBars)
}

// TestExportFormUnsetNumbersKeepFormDefaults checks that a field the form
// never filled in is omitted rather than written as a zero, because zero means
// "unset" to the GIF bounds check and must keep meaning that after a round trip.
func TestExportFormUnsetNumbersKeepFormDefaults(t *testing.T) {
	t.Parallel()

	location := previewRedirectURL(
		"42",
		"preview-1",
		0,
		5,
		nil,
		exportFormState{Type: clipTypeClip},
	)

	parsed, err := url.Parse(location)
	require.NoError(t, err)

	query := parsed.Query()
	assert.NotContains(t, query, queryWidth)
	assert.NotContains(t, query, queryFPS)
	assert.NotContains(t, query, queryAudioIndex)

	got := exportFormFromTarget(t, location, false)
	assert.Zero(t, got.Width)
	assert.Zero(t, got.FPS)
	assert.Zero(t, got.AudioIndex)
}

// TestExportFormRejectsUnknownQueryValues covers a hand-edited or stale
// redirect. A value the form does not offer must not reach the select, and a
// malformed number must not become a nonzero export setting.
func TestExportFormRejectsUnknownQueryValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		query     string
		wantType  string
		wantWidth int
	}{
		{name: "unknown export type falls back", query: "exportType=avi", wantType: clipTypeClip},
		{name: "empty export type falls back", query: "exportType=", wantType: clipTypeClip},
		{
			name:     "valid export type is kept",
			query:    "exportType=screenshot",
			wantType: clipTypeScreenshot,
		},
		{
			name:      "non numeric width is unset",
			query:     "width=wide",
			wantType:  clipTypeClip,
			wantWidth: 0,
		},
		{
			name:      "negative width is kept verbatim",
			query:     "width=-5",
			wantType:  clipTypeClip,
			wantWidth: -5,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := exportFormFromTarget(t, "/media/item/42?"+test.query, false)

			assert.Equal(t, test.wantType, got.Type)
			assert.Equal(t, test.wantWidth, got.Width)
		})
	}
}

// TestExportFormStateLeavesMarksAlone guards against the carry widening into
// the timecodes, which have their own precision contract and their own tests.
func TestExportFormStateLeavesMarksAlone(t *testing.T) {
	t.Parallel()

	req := api.ClipRequest{
		MediaID:       "42",
		StartTime:     1,
		Duration:      2,
		ClipType:      clipTypeGIF,
		Width:         640,
		CropBlackBars: true,
	}

	location := previewRedirectURL(
		req.MediaID,
		"preview-1",
		req.StartTime,
		req.StartTime+req.Duration,
		req.WebSafeColor,
		exportFormFromRequest(req),
	)

	parsed, err := url.Parse(location)
	require.NoError(t, err)

	assert.Equal(t, "1.000", parsed.Query().Get("start"))
	assert.Equal(t, "3.000", parsed.Query().Get("end"))
}

// TestExportFormKeepsClearedName covers a name the user deliberately emptied.
// The redirect has to write the field so the clear survives; otherwise the page
// refills it from the title and the next submit saves the title.
func TestExportFormKeepsClearedName(t *testing.T) {
	t.Parallel()

	req := api.ClipRequest{
		MediaID:  "42",
		ClipType: clipTypeClip,
		Name:     "",
	}

	location := previewRedirectURL(
		req.MediaID,
		"preview-1",
		0,
		5,
		nil,
		exportFormFromRequest(req),
	)

	parsed, err := url.Parse(location)
	require.NoError(t, err)

	assert.Contains(t, parsed.Query(), queryExportName, "an empty name must still be carried")

	assert.Empty(t, exportFormFromTarget(t, location, false).Name)
}

// TestExportFormAbsentNameFallsBackToTitle covers a first visit, which carries
// no query at all and should show the media title. A preview redirect always
// carries the name, empty or not, because the form always submits it.
func TestExportFormAbsentNameFallsBackToTitle(t *testing.T) {
	t.Parallel()

	assert.Equal(t, testMovie, exportFormFromTarget(t, "/media/item/42", false).Name)
	assert.Equal(
		t,
		"Some Other Title",
		exportFormFromTargetWithTitle(t, "/media/item/42", false, "Some Other Title").Name,
	)
}

// TestExportFormNameSurvivesTitleThatLooksEmpty covers a source whose display
// title is blank. The carried name must win over the fallback, and the
// fallback must not be mistaken for a deliberate clear.
func TestExportFormNameSurvivesTitleThatLooksEmpty(t *testing.T) {
	t.Parallel()

	location := previewRedirectURL(
		"42",
		"preview-1",
		0,
		5,
		nil,
		exportFormState{Name: "Kept"},
	)

	assert.Equal(t, "Kept", exportFormFromTargetWithTitle(t, location, false, "").Name)
}

func TestNormalizeExportType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, clipTypeClip, normalizeExportType(""))
	assert.Equal(t, clipTypeClip, normalizeExportType("CLIP"))
	assert.Equal(t, clipTypeClip, normalizeExportType("video"))
	assert.Equal(t, clipTypeGIF, normalizeExportType(clipTypeGIF))
	assert.Equal(t, clipTypeScreenshot, normalizeExportType(clipTypeScreenshot))
}

func TestDerefBoolStillPrefersExplicitFalse(t *testing.T) {
	t.Parallel()

	assert.False(t, derefBool(nil))
	assert.True(t, derefBool(new(true)))
	assert.False(t, derefBool(new(false)))
}
