// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	domainclip "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestClipCardPollDoesNotCoverEditForm(t *testing.T) {
	t.Parallel()

	item := activeTestItem()

	var buf strings.Builder

	require.NoError(t, ClipCard(item).Render(t.Context(), &buf))
	body := buf.String()

	assert.Contains(t, body, `<div id="clip-c1">`)

	statusAt := strings.Index(body, `id="clip-c1-status"`)
	require.Positive(t, statusAt, "the polled status region must exist")

	assert.Contains(t, body, `hx-get="/clips/c1/row"`)
	assert.Contains(t, body, `hx-trigger="every 2s"`)
	assert.Contains(t, body, `hx-swap="outerHTML"`)

	gridAt := strings.Index(body, `<div class="grid gap-4 sm:grid-cols-2">`)
	require.Positive(t, gridAt)
	assert.Less(t, statusAt, gridAt, "the polled region must close before the form fields")

	cancelAt := strings.Index(body, "Cancel")
	require.Positive(t, cancelAt)
	assert.Less(t, cancelAt, gridAt, "the action buttons must sit inside the polled region")

	finished := activeTestItem()
	finished.Status = domainclip.StatusCompleted
	finished.FileExists = true

	var finishedBuf strings.Builder

	require.NoError(t, ClipCard(finished).Render(t.Context(), &finishedBuf))
	finishedBody := finishedBuf.String()

	finishedStatusAt := strings.Index(finishedBody, `id="clip-c1-status"`)
	finishedGridAt := strings.Index(finishedBody, `<div class="grid gap-4 sm:grid-cols-2">`)
	require.Positive(t, finishedStatusAt)
	require.Positive(t, finishedGridAt)

	for _, action := range []string{"Regenerate", "Save metadata", "/api/clips/c1/download"} {
		at := strings.Index(finishedBody, action)
		require.Positive(t, at, action)
		assert.Less(t, at, finishedGridAt, action+" must sit inside the polled region")
	}

	assertGridOutsideStatusRegion(t, "encoding", body)
	assertGridOutsideStatusRegion(t, "finished", finishedBody)

	formAt := strings.Index(body, `action="/api/clips/c1/update"`)
	require.Positive(t, formAt)
	assert.Less(t, formAt, statusAt, "the status region is a sibling of the form fields, inside the form")
}

func assertGridOutsideStatusRegion(t *testing.T, label, body string) {
	t.Helper()

	doc, err := html.Parse(strings.NewReader(body))
	require.NoError(t, err, label)

	region := findByID(doc, "clip-c1-status")
	require.NotNil(t, region, "%s: the polled status region must exist", label)

	grid := findByClass(doc, "grid gap-4 sm:grid-cols-2")
	require.NotNil(t, grid, "%s: the form fields grid must exist", label)

	assert.False(t, containsNode(region, grid),
		"%s: the edit form must not be inside the polled region", label)
}

func findByID(node *html.Node, id string) *html.Node {
	if node.Type == html.ElementNode && attrValue(node, "id") == id {
		return node
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findByID(child, id); found != nil {
			return found
		}
	}

	return nil
}

func findByClass(node *html.Node, class string) *html.Node {
	if node.Type == html.ElementNode && attrValue(node, "class") == class {
		return node
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findByClass(child, class); found != nil {
			return found
		}
	}

	return nil
}

func containsNode(ancestor, candidate *html.Node) bool {
	for child := ancestor.FirstChild; child != nil; child = child.NextSibling {
		if child == candidate || containsNode(child, candidate) {
			return true
		}
	}

	return false
}

func attrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}

	return ""
}

func TestClipStatusSwapsActionButtons(t *testing.T) {
	t.Parallel()

	encoding := activeTestItem()

	var active strings.Builder

	require.NoError(t, ClipStatus(encoding).Render(t.Context(), &active))

	activeBody := active.String()
	assert.Contains(t, activeBody, "Cancel")
	assert.NotContains(t, activeBody, "Regenerate", "an encoding clip cannot be regenerated")
	assert.NotContains(t, activeBody, "Download")

	done := activeTestItem()
	done.Status = domainclip.StatusCompleted
	done.FileExists = true

	var finished strings.Builder

	require.NoError(t, ClipStatus(done).Render(t.Context(), &finished))

	finishedBody := finished.String()
	assert.NotContains(t, finishedBody, "Cancel", "a finished clip cannot be cancelled")
	assert.Contains(t, finishedBody, "Regenerate")
	assert.Contains(t, finishedBody, "Save metadata")
	assert.Contains(t, finishedBody, "/api/clips/c1/download")
}

func TestClipStatusCarriesEverythingThatChanges(t *testing.T) {
	t.Parallel()

	progressing := activeTestItem()
	progressing.Progress = 40
	progressing.Error = "boom"

	var active strings.Builder

	require.NoError(t, ClipStatus(progressing).Render(t.Context(), &active))

	activeBody := active.String()
	assert.Contains(t, activeBody, string(domainclip.StatusProcessing))
	assert.Contains(t, activeBody, "boom")
	assert.Contains(t, activeBody, "40%")
	assert.Contains(t, activeBody, `hx-get="/clips/c1/row"`)
	assert.NotContains(t, activeBody, "Preview", "no player while the clip is still encoding")

	done := activeTestItem()
	done.Status = domainclip.StatusCompleted
	done.Progress = 100
	done.FileExists = true

	var finished strings.Builder

	require.NoError(t, ClipStatus(done).Render(t.Context(), &finished))

	finishedBody := finished.String()
	assert.Contains(t, finishedBody, "Preview")
	assert.Contains(t, finishedBody, "/clips/c1/file")
	assert.NotContains(t, finishedBody, "100%", "the progress bar is gone once complete")
	assert.NotContains(t, finishedBody, `hx-trigger`, "a finished clip stops polling itself")
}

func activeTestItem() view.ClipItem {
	return view.ClipItem{
		ID:          "c1",
		Name:        "Intro",
		MediaID:     "42",
		MediaTitle:  "Movie",
		ClipType:    domainclip.TypeClip,
		Status:      domainclip.StatusProcessing,
		Progress:    40,
		StartTime:   time.Second,
		Duration:    5 * time.Second,
		Quality:     "archive",
		ProfileName: "Archive",
		Profiles: []profile.ProfileOption{
			{ID: "archive", Name: "Archive"},
		},
		FileExists: false,
		MaxDur:     10 * time.Minute,
	}
}

func TestClipCard(t *testing.T) {
	t.Parallel()

	base := view.ClipItem{
		ID:          "c1",
		Name:        "Intro",
		MediaID:     "42",
		MediaTitle:  "Movie",
		ClipType:    domainclip.TypeClip,
		Status:      domainclip.StatusCompleted,
		Progress:    100,
		CreatedAt:   "Jan 1, 2026 12:00 AM",
		StartTime:   time.Second,
		Duration:    5 * time.Second,
		Quality:     "archive",
		ProfileName: "Archive",
		Profiles: []profile.ProfileOption{
			{ID: "archive", Name: "Archive", IsDefault: false},
		},
		FileExists:    true,
		AudioIndex:    0,
		AudioTracks:   nil,
		CropBlackBars: false,
		MaxDur:        10 * time.Minute,
	}

	tests := []struct {
		name        string
		tweak       func(*view.ClipItem)
		contains    []string
		notContains []string
	}{
		{
			name: "completed with file",
			contains: []string{
				"Archive",
				"Video clip",
				"/clips/c1/file",
				"Regenerate",
				"<details",
				"Preview",
				"<video",
				"hx-confirm",
				`hx-disable="this"`,
				`name="endTime"`,
				`name="cropBlackBars"`,
				`name="webSafeColor"`,
				`data-max-dur=`,
			},
			notContains: []string{"<details open", "On disk"},
		},
		{
			name: "falls back to media title",
			tweak: func(item *view.ClipItem) {
				item.Name = ""
			},
			contains: []string{
				"Movie",
				`name="name"`,
				`value="Movie"`,
			},
		},
		{
			name: "missing file",
			tweak: func(item *view.ClipItem) {
				item.FileExists = false
			},
			contains: []string{"Missing file"},
			notContains: []string{
				"On disk",
				"<video",
				"<details",
			},
		},
		{
			name: "failed status",
			tweak: func(item *view.ClipItem) {
				item.Status = "failed"
				item.FileExists = false
				item.Error = "ffmpeg exited 1"
			},
			contains:    []string{"failed", "Missing file", "ffmpeg exited 1"},
			notContains: []string{"On disk", "hx-trigger"},
		},
		{
			name: "canceled status",
			tweak: func(item *view.ClipItem) {
				item.Status = domainclip.StatusCancelled
				item.FileExists = false
			},
			contains: []string{string(domainclip.StatusCancelled)},
			notContains: []string{
				"Missing file",
				"On disk",
				"hx-trigger",
			},
		},
		{
			name: "gif preview",
			tweak: func(item *view.ClipItem) {
				item.ClipType = domainclip.TypeGIF
				item.Width = 640
				item.FPS = 12
			},
			contains: []string{
				"<img",
				"/clips/c1/file",
				"Preview",
				`name="width"`,
				`name="fps"`,
				`name="endTime"`,
				`value="640"`,
				`value="12"`,
				"GIF width (px)",
				`name="cropBlackBars"`,
			},
			notContains: []string{"<video"},
		},
		{
			name: "screenshot preview",
			tweak: func(item *view.ClipItem) {
				item.ClipType = domainclip.TypeScreenshot
			},
			contains: []string{
				"<img",
				"/clips/c1/file",
				"Time",
				`name="startTime"`,
				`name="cropBlackBars"`,
				`data-export-for="screenshot"`,
				`col-start-1 row-start-1`,
			},
			notContains: []string{"<video"},
		},
		{
			name: "active polling hides preview",
			tweak: func(item *view.ClipItem) {
				item.Status = domainclip.StatusProcessing
				item.Progress = 40
				item.FileExists = false
			},
			contains: []string{
				`hx-get="/clips/c1/row"`,
				`hx-trigger="every 2s"`,
				`hx-disable="this"`,
				"40%",
				"Cancel",
			},
			notContains: []string{
				"Preview",
				"<video",
				"<img",
				"On disk",
				"Missing file",
			},
		},
		{
			name: "audio tracks",
			tweak: func(item *view.ClipItem) {
				item.AudioTracks = []view.AudioTrackOption{
					{Index: 0, Label: "eng · aac"},
				}
			},
			contains: []string{`name="audioIndex"`, "eng · aac"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			item := base
			if test.tweak != nil {
				test.tweak(&item)
			}

			var buf strings.Builder

			err := ClipCard(item).Render(t.Context(), &buf)
			require.NoError(t, err)

			body := buf.String()
			for _, want := range test.contains {
				assert.Contains(t, body, want)
			}

			for _, hide := range test.notContains {
				assert.NotContains(t, body, hide)
			}

			typeIdx := strings.Index(body, `name="clipType"`)
			nameIdx := strings.Index(body, `id="name-c1"`)
			if typeIdx >= 0 && nameIdx >= 0 {
				assert.Less(t, typeIdx, nameIdx)
			}
		})
	}
}

func TestListToolbar(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := ListToolbar(ListToolbarProps{
		Action:  "/clips",
		Target:  "clip-list",
		PushURL: true,
		Status:  domainclip.StatusCompleted,
		Type:    domainclip.TypeGIF,
		Query:   "intro",
		Sort:    "name_asc",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `hx-get="/clips"`)
	assert.Contains(t, body, `hx-target="#clip-list"`)
	assert.Contains(t, body, `hx-push-url="true"`)
	assert.Contains(t, body, `hx-trigger="change from:select, submit"`)
	assert.Contains(t, body, `name="status"`)
	assert.Contains(t, body, `value="completed"`)
	assert.Contains(t, body, `name="type"`)
	assert.Contains(t, body, `name="q"`)
	assert.Contains(t, body, `name="sort"`)
	assert.Contains(t, body, `value="intro"`)
	assert.Contains(t, body, "Filter")
}
