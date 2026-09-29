// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestMediaItemPageLoadsExternalScript(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:          "42",
		Title:       testMovie,
		Type:        "",
		Duration:    0,
		MaxDur:      600,
		Clips:       nil,
		ClipStatus:  "",
		ClipType:    exportTypeGIF,
		ClipQuery:   "intro",
		ClipSort:    "name_asc",
		Profiles:    nil,
		AudioTracks: nil,
		Error:       "",
		PreviewID:   "",
		StartTime:   0,
		EndTime:     0,
		Export: view.ExportForm{
			WebSafeColor: true,
		},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `src="/assets/js/media-item.js"`)
	assert.Contains(t, body, `data-max-dur="600"`)
	assert.Contains(t, body, `name="endTime"`)
	assert.Contains(t, body, "Time")
	assert.Contains(t, body, `name="cropBlackBars"`)
	assert.Contains(t, body, `name="webSafeColor"`)
	assert.Contains(t, body, "Web-safe color")
	assert.Contains(t, body, `name="webSafeColor" value="1" checked`)
	assert.Less(t, strings.Index(body, `id="clipType"`), strings.Index(body, `id="name"`))
	assert.NotContains(t, body, "Start (seconds)")
	assert.NotContains(t, body, "formatTimecode")
	assert.Contains(t, body, `hx-get="/media/item/42/clips"`)
	assert.Contains(t, body, `hx-target="#item-clip-list"`)
	assert.NotContains(t, body, `hx-push-url`)
	assert.Contains(t, body, `id="clip-list-type"`)
	assert.Contains(t, body, `value="intro"`)
}

func TestMediaItemPagePreservesStatusFilter(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:          "42",
		Title:       testMovie,
		Type:        "",
		Duration:    0,
		MaxDur:      600,
		Clips:       nil,
		ClipStatus:  "pending",
		ClipType:    "",
		ClipQuery:   "",
		ClipSort:    "created_desc",
		Profiles:    nil,
		AudioTracks: nil,
		Error:       "",
		PreviewID:   "",
		StartTime:   0,
		EndTime:     0,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `name="status"`)
	assert.Contains(t, body, `value="pending"`)
	assert.Contains(t, body, "No clips match these filters.")
	assert.NotContains(t, body, "No clips yet.")
}

// TestMediaItemPageRendersCarriedExportForm covers the rendered half of the
// preview round trip. The handlers tests prove the state survives the
// redirect; this proves the form comes back showing it.
func TestMediaItemPageRendersCarriedExportForm(t *testing.T) {
	t.Parallel()

	profiles := []view.ClipProfileOption{
		{ID: "profile-low", Name: "Low", IsDefault: true},
		{ID: "profile-high", Name: "High"},
	}
	tracks := []view.AudioTrackOption{
		{Index: 0, Label: "English"},
		{Index: 2, Label: "Commentary"},
	}

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:          "42",
		Title:       testMovie,
		MaxDur:      600,
		Profiles:    profiles,
		AudioTracks: tracks,
		StartTime:   10.345,
		EndTime:     18.007,
		Export: view.ExportForm{
			Type:          exportTypeGIF,
			Name:          "A named clip",
			Quality:       "profile-high",
			AudioIndex:    2,
			Width:         1280,
			FPS:           24,
			CropBlackBars: true,
			WebSafeColor:  true,
		},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `<option value="gif" selected>`)
	assert.Contains(t, body, `name="name" placeholder="Optional name" value="A named clip"`)
	assert.Contains(t, body, `<option value="profile-high" selected>`)
	assert.Contains(t, body, `<option value="2" selected>`)
	assert.Contains(t, body, `name="cropBlackBars" value="1" checked`)
	assert.Contains(t, body, `name="webSafeColor" value="1" checked`)
	assert.Contains(t, body, `name="width" type="number" min="120" max="1920" value="1280"`)
	assert.Contains(t, body, `name="fps" type="number" min="5" max="30" value="24"`)
}

// TestMediaItemPageFallsBackToFormDefaults covers a first visit, where nothing
// was carried and the form should show its own defaults rather than blanks.
// TestMediaItemPageFallsBackToFormDefaults covers a first visit, where nothing
// was carried and the form should show its own defaults. The clip name arrives
// already resolved to the media title, because the handler owns that fallback.
func TestMediaItemPageFallsBackToFormDefaults(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  testMovie,
		MaxDur: 600,
		Export: view.ExportForm{Name: testMovie},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `<option value="clip" selected>`)
	assert.Contains(t, body, `name="name" placeholder="Optional name" value="`+testMovie+`"`)
	assert.Contains(t, body, `name="width" type="number" min="120" max="1920" value="480"`)
	assert.Contains(t, body, `name="fps" type="number" min="5" max="30" value="10"`)
	assert.NotContains(t, body, `name="cropBlackBars" value="1" checked`)
	assert.NotContains(t, body, `name="webSafeColor" value="1" checked`)
}

// TestMediaItemPageRendersClearedName covers a name the user deliberately
// emptied. The form must show it empty rather than falling back to the media
// title, so the next submit stays clear. An input with no value attribute
// submits an empty field, so the attribute is expected to be absent.
func TestMediaItemPageRendersClearedName(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  testMovie,
		MaxDur: 600,
		Export: view.ExportForm{Name: ""},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `name="name" placeholder="Optional name" class=`)
	assert.NotContains(t, body, `name="name" placeholder="Optional name" value=`)
	assert.Contains(t, body, `name="mediaTitle" value="`+testMovie+`"`)
}

// TestMediaItemPageRendersPreviewIndicator covers the elements the preview
// progress script drives, and that they come from the shared components rather
// than hand-rolled markup.
//
// The script looks each of these up by id, so a missing one silently disables
// that part of the indicator rather than failing loudly.
func TestMediaItemPageRendersPreviewIndicator(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:        "42",
		Title:     testMovie,
		MaxDur:    600,
		PreviewID: "abc123",
		Export:    view.ExportForm{},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	for _, id := range []string{
		"preview-video",
		"preview-progress",
		"preview-cancel",
		"preview-status",
		"preview-button",
		"preview-button-label",
	} {
		assert.Contains(t, body, `id="`+id+`"`, "the preview script needs #"+id)
	}

	// The progress component's own script drives the bar from aria-valuenow, so
	// both the hook and the attribute it watches must be present.
	assert.Contains(t, body, "data-tui-progress-indicator")
	assert.Contains(t, body, `role="progressbar"`)
	assert.Contains(t, body, `aria-valuenow="0"`)

	// The component script must be requested, at the path the app actually
	// serves. The base path is what makes the other component scripts reachable
	// at all, so a regression here is silent until a component needs one.
	assert.Contains(t, body, `src="/assets/js/progress.min.js`)

	// The player must not point at a file that has not been published yet.
	assert.NotContains(t, body, `src="/previews/`)

	// The cancel control comes from the button component, which is what makes it
	// legible once the script disables it while a render is underway. A bare
	// button would keep the enabled styling in that state, so the check is
	// scoped to the cancel button's own tag rather than the page.
	cancelTag := regexp.MustCompile(`<button[^>]*id="preview-cancel"[^>]*>`).FindString(body)
	require.NotEmpty(t, cancelTag, "the cancel control should be a button")
	assert.Contains(t, cancelTag, "disabled:pointer-events-none")

	// Cancel must be an htmx request, not a bare fetch. The layout puts the CSRF
	// token on hx-headers:inherited, so only htmx carries it; a fetch would be
	// rejected and the cancel would silently do nothing.
	assert.Contains(t, cancelTag, `hx-delete="/api/clips/preview/abc123"`)
	assert.Contains(t, cancelTag, `hx-swap="none"`, "the JSON reply must not replace the button")

	// Both start hidden, so nothing is shown before a render is underway. The
	// script reveals the cancel control when a render starts, and the set-end
	// control once the player can be seeked.
	assert.Regexp(t, `id="preview-progress"[^>]*class="[^"]*hidden`, body)
	assert.Contains(t, cancelTag, "hidden")
	assert.Regexp(t, `id="preview-set-end"[^>]*class="[^"]*hidden`, body)

	// The proxy carries where it starts, so a position in it can be mapped back
	// to a source time.
	assert.Contains(t, body, `data-preview-start="`)
}

// TestMediaItemPageOmitsPreviewIndicatorWithoutAPreview covers the page without
// a preview in flight, which should render no indicator at all.
func TestMediaItemPageOmitsPreviewIndicatorWithoutAPreview(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  testMovie,
		MaxDur: 600,
		Export: view.ExportForm{},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.NotContains(t, body, `id="preview-progress"`)
	assert.NotContains(t, body, `id="preview-video"`)
	// The submit button stays, since it is how a preview is started.
	assert.Contains(t, body, `id="preview-button"`)
}

// TestItemClipListOmitsLayout keeps the clip list free of the full page chrome
// so it can be swapped into a page that already has it.
func TestItemClipListOmitsLayout(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ItemClipList(nil, true).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "No clips match these filters.")
	assert.NotContains(t, body, "Outtake")
	assert.NotContains(t, body, `id="clip-list-type"`)
}
