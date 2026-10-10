// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/web/assets"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func inputTagFor(t *testing.T, body, name string) string {
	t.Helper()

	pattern := `<input[^>]*name="` + regexp.QuoteMeta(name) + `"[^>]*>`
	tag := regexp.MustCompile(pattern).FindString(body)
	require.NotEmpty(t, tag, "no <input> carrying name=%q in the rendered page", name)

	return tag
}

func TestMediaItemPageLoadsExternalScript(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:          "42",
		Title:       "Movie",
		Type:        "",
		Duration:    0,
		MaxDur:      600 * time.Second,
		Clips:       nil,
		ClipStatus:  "",
		ClipType:    clip.TypeGIF,
		ClipQuery:   "intro",
		ClipSort:    "name_asc",
		Profiles:    nil,
		AudioTracks: nil,
		Error:       "",
		PreviewID:   "",
		StartTime:   0,
		EndTime:     0,
		Export:      view.ExportForm{},
		SourceHDR:   true,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `src="`+assets.URL("js/media-item.js")+`"`)
	assert.Contains(t, body, `data-max-dur="600"`)
	assert.Contains(t, body, `name="endTime"`)
	assert.Contains(t, body, "Time")
	assert.Contains(t, body, `name="cropBlackBars"`)
	assert.NotContains(t, body, `name="webSafeColor"`)
	assert.NotContains(t, body, `name="preserveHdr"`, "Keep HDR is the chosen profile's setting")
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
		Title:       "Movie",
		Type:        "",
		Duration:    0,
		MaxDur:      600 * time.Second,
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
	assert.Contains(t, body, "No clips match these filters")
	assert.NotContains(t, body, "No clips yet")
}

func TestMediaItemPageRendersCarriedExportForm(t *testing.T) {
	t.Parallel()

	profiles := []profile.ProfileOption{
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
		Title:       "Movie",
		MaxDur:      600 * time.Second,
		Profiles:    profiles,
		AudioTracks: tracks,
		StartTime:   10345 * time.Millisecond,
		EndTime:     18007 * time.Millisecond,
		Export: view.ExportForm{
			Type:          clip.TypeGIF,
			Name:          "A named clip",
			Quality:       "profile-high",
			AudioIndex:    2,
			Width:         1280,
			FPS:           24,
			CropBlackBars: true,
		},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `<option value="gif" selected>`)
	assert.Contains(t, body, `name="name" placeholder="Optional" value="A named clip"`)
	assert.Contains(t, body, `<option value="profile-high" selected>`)
	assert.Contains(t, body, `<option value="2" selected>`)

	cropBars := inputTagFor(t, body, "cropBlackBars")
	assert.Contains(t, cropBars, `value="1"`)
	assert.Contains(t, cropBars, "checked")

	assert.Contains(t, body, `name="width" type="number" min="120" max="1920" value="1280"`)
	assert.Contains(t, body, `name="fps" type="number" min="5" max="30" value="24"`)
}

func TestMediaItemPageFallsBackToFormDefaults(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  "Movie",
		MaxDur: 600 * time.Second,
		Export: view.ExportForm{Name: "Movie"},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `<option value="clip" selected>`)
	assert.Contains(t, body, `name="name" placeholder="Optional" value="Movie"`)
	assert.Contains(t, body, `name="width" type="number" min="120" max="1920" value="480"`)
	assert.Contains(t, body, `name="fps" type="number" min="5" max="30" value="10"`)
	assert.NotContains(t, body, `name="cropBlackBars" value="1" checked`)
	assert.NotContains(t, body, `name="preserveHdr"`, "no export form offers a Keep HDR box")
}

func TestMediaItemPageRendersClearedName(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  "Movie",
		MaxDur: 600 * time.Second,
		Export: view.ExportForm{Name: ""},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, `name="name" placeholder="Optional" class=`)
	assert.NotContains(t, body, `name="name" placeholder="Optional" value=`)
	assert.Contains(t, body, `name="mediaTitle" value="Movie"`)
}

func TestMediaItemPageRendersPreviewIndicator(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:        "42",
		Title:     "Movie",
		MaxDur:    600 * time.Second,
		PreviewID: "abc123",
		Export:    view.ExportForm{},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	for _, id := range []string{
		"preview-video",
		"preview-format",
		"preview-progress",
		"preview-cancel",
		"preview-status",
		"preview-button",
		"preview-button-label",
	} {
		assert.Contains(t, body, `id="`+id+`"`, "the preview script needs #"+id)
	}

	assert.Contains(t, body, "data-player-frame", "the format badge sits in the player's frame")
	assert.Regexp(t, `id="preview-format"[^>]*\shidden[\s>]`, body,
		"the badge stays hidden until the script fills it in")

	assert.Contains(t, body, "data-tui-progress-indicator")
	assert.Contains(t, body, `role="progressbar"`)
	assert.Contains(t, body, `aria-valuenow="0"`)

	assert.Contains(t, body, `src="`+assets.URL("js/progress.min.js")+`"`)

	assert.NotContains(t, body, `src="/previews/`)

	cancelTag := regexp.MustCompile(`<button[^>]*id="preview-cancel"[^>]*>`).FindString(body)
	require.NotEmpty(t, cancelTag, "the cancel control should be a button")
	assert.Contains(t, cancelTag, "disabled:pointer-events-none")

	assert.Contains(t, cancelTag, `hx-delete="/api/clips/preview/abc123"`)
	assert.Contains(t, cancelTag, `hx-swap="none"`, "the JSON reply must not replace the button")

	assert.Regexp(t, `id="preview-progress"[^>]*class="[^"]*hidden`, body)
	assert.Contains(t, cancelTag, "hidden")
	assert.Regexp(t, `id="preview-set-end"[^>]*class="[^"]*hidden`, body)

	assert.Contains(t, body, `data-preview-start="`)
}

func TestMediaItemPageOmitsPreviewIndicatorWithoutAPreview(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  "Movie",
		MaxDur: 600 * time.Second,
		Export: view.ExportForm{},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.NotContains(t, body, `id="preview-progress"`)
	assert.NotContains(t, body, `id="preview-video"`)
	assert.Contains(t, body, `id="preview-button"`)
}

func TestItemClipListOmitsLayout(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ItemClipList(nil, true).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "No clips match these filters")
	assert.NotContains(t, body, "Outtake")
	assert.NotContains(t, body, `id="clip-list-type"`)
}

func TestMediaItemPageFallsBackToTheLibraryLink(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID: "42", Title: "Movie", MaxDur: 600 * time.Second,
	}).
		Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(
		t,
		body,
		`<a href="/media" class="text-sm text-muted-foreground `+`hover:text-foreground">Media Libraries</a>`,
	)
	assert.NotContains(t, body, "<nav")
}

func TestMediaItemPageRendersTheBreadcrumbTrail(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  "Movie",
		MaxDur: 600 * time.Second,
		Crumbs: []view.Crumb{
			{Title: "Movies", URL: "/media?library=1"},
			{Title: "Action"},
			{Title: "Movie"},
		},
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `<a href="/media?library=1" class="hover:text-foreground">Movies</a>`)
	assert.Equal(t, 2, strings.Count(body, `<span>/</span>`),
		"each crumb after the first is separated")
	assert.Contains(t, body, `<span class="text-foreground">Action</span>`,
		"a crumb with no link is the current page rather than a link")
	assert.Contains(t, body, `<span class="text-foreground">Movie</span>`)
	assert.NotContains(
		t,
		body,
		`<a href="/media" class="text-sm text-muted-foreground `+`hover:text-foreground">Media Libraries</a>`,
		"the breadcrumb trail replaces the bare library link",
	)
}

func TestItemClipListRendersEveryClip(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ItemClipList([]view.ClipItem{
		{ID: "clip-one", Name: "Opening"},
		{ID: "clip-two", Name: "Closing"},
	}, false).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `id="clip-clip-one"`)
	assert.Contains(t, body, `id="clip-clip-two"`)
	assert.Contains(t, body, "Opening")
	assert.Contains(t, body, "Closing")
	assert.NotContains(t, body, "No clips match these filters")
}

func TestMediaItemPageShowsTheSaveError(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:     "42",
		Title:  "Movie",
		MaxDur: 600 * time.Second,
		Error:  "That clip name is already taken.",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "That clip name is already taken.")
	assert.Contains(t, body, "js-flash")
}

// TestMediaItemPageOffersNoKeepHDRControl covers Keep HDR as a profile
// setting: neither an HDR nor an SDR source offers the box, and the header
// still names the source's quality.
func TestMediaItemPageOffersNoKeepHDRControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sourceHDR bool
		quality   string
	}{
		{name: "an hdr source", sourceHDR: true, quality: "4K HDR10"},
		{name: "an sdr source", sourceHDR: false, quality: "1080p"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			err := MediaItemPage(MediaItemPageProps{
				ID:        "42",
				Title:     "Movie",
				Type:      "movie",
				Quality:   test.quality,
				SourceHDR: test.sourceHDR,
				MaxDur:    600 * time.Second,
			}).Render(t.Context(), &buf)
			require.NoError(t, err)

			body := buf.String()

			assert.Contains(t, body, test.quality, "the header carries the source quality")
			assert.NotContains(t, body, `name="preserveHdr"`)
		})
	}
}

func TestMediaItemPageShowsAShortDuration(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:       "42",
		Title:    "Movie",
		Type:     "movie",
		Duration: 7325 * time.Second,
		MaxDur:   600 * time.Second,
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	assert.Contains(t, body, "2hr2min5s")
	assert.NotContains(t, body, "02:02:05")
}

// TestMediaItemPageCarriesTheScreenHDRField covers the export form's screen
// flag: the form carries a field the script marks on an HDR screen, so a
// preview is tone mapped only where HDR cannot be shown.
func TestMediaItemPageCarriesTheScreenHDRField(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := MediaItemPage(MediaItemPageProps{
		ID:        "42",
		Title:     "Movie",
		MaxDur:    600 * time.Second,
		PreviewID: "preview-1",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), `name="screenHdr" value="0" data-screen-hdr`)
}
