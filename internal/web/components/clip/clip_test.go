// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

// TestClipCardPollDoesNotCoverEditForm is the regression guard for the
// destructive poll.
//
// The card root must stay a bare element, because the poll swaps the status
// region with outerHTML and anything inside that region is replaced. If the
// poll attributes were moved back onto the card root, the whole card including
// the edit form would be re-rendered every two seconds and discard whatever the
// user had typed.
func TestClipCardPollDoesNotCoverEditForm(t *testing.T) {
	t.Parallel()

	item := activeTestItem()

	var buf strings.Builder

	require.NoError(t, ClipCard(item).Render(t.Context(), &buf))
	body := buf.String()

	// A bare root proves no poll attributes leaked onto the card.
	assert.Contains(t, body, `<div id="clip-c1">`)

	statusAt := strings.Index(body, `id="clip-c1-status"`)
	require.Positive(t, statusAt, "the polled status region must exist")

	// The region carries the poll.
	assert.Contains(t, body, `hx-get="/clips/c1/row"`)
	assert.Contains(t, body, `hx-trigger="every 2s"`)
	assert.Contains(t, body, `hx-swap="outerHTML"`)

	// templ emits well-formed markup, so the region closing before the form
	// fields grid opens proves the editable inputs sit outside the region and
	// therefore survive the swap.
	gridAt := strings.Index(body, `<div class="grid gap-4 sm:grid-cols-2">`)
	require.Positive(t, gridAt)
	assert.Less(t, statusAt, gridAt, "the polled region must close before the form fields")

	// The action buttons change with the clip, so they belong in the region.
	// Leaving them outside strands a finished clip showing Cancel.
	cancelAt := strings.Index(body, "Cancel")
	require.Positive(t, cancelAt)
	assert.Less(t, cancelAt, gridAt, "the action buttons must sit inside the polled region")

	// A finished card offers the other actions, and they must also be inside
	// the region for the same reason.
	finished := activeTestItem()
	finished.Status = view.ClipStatusCompleted
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

	// The same containment check, parsed, for both card states. This is the
	// assertion that actually matters: the poll replaces its target's subtree,
	// so a form field anywhere inside the region is destroyed every two seconds.
	assertGridOutsideStatusRegion(t, "encoding", body)
	assertGridOutsideStatusRegion(t, "finished", finishedBody)

	formAt := strings.Index(body, `action="/api/clips/c1/update"`)
	require.Positive(t, formAt)
	assert.Less(t, formAt, statusAt, "the status region is a sibling of the form fields, inside the form")
}

// assertGridOutsideStatusRegion parses rendered card markup and asserts the
// editable form fields are not a descendant of the polled status region.
//
// It checks containment rather than source order, because that is what the
// htmx swap actually does: everything inside the region is replaced.
//
// Parameters:
//   - t: Test context.
//   - label: Card state, used to make failures readable.
//   - body: Rendered card markup.
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

// findByID returns the first element with the given id attribute.
//
// Parameters:
//   - node: Node to search from.
//   - id: Element id to match.
//
// Returns:
//   - found: The matching element, or nil.
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

// findByClass returns the first element carrying every given class.
//
// Parameters:
//   - node: Node to search from.
//   - class: Space separated class list to match.
//
// Returns:
//   - found: The matching element, or nil.
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

// containsNode reports whether candidate sits inside ancestor's subtree.
//
// Parameters:
//   - ancestor: Possible ancestor.
//   - candidate: Node to look for.
//
// Returns:
//   - found: True when candidate is a strict descendant of ancestor.
func containsNode(ancestor, candidate *html.Node) bool {
	for child := ancestor.FirstChild; child != nil; child = child.NextSibling {
		if child == candidate || containsNode(child, candidate) {
			return true
		}
	}

	return false
}

// attrValue reads an attribute value.
//
// Parameters:
//   - node: Element to read from.
//   - key: Attribute name.
//
// Returns:
//   - value: The attribute value, or an empty string when absent.
func attrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}

	return ""
}

// TestClipStatusSwapsActionButtons pins the action buttons into the polled
// region, since which actions are offered depends on the clip status: an
// encoding clip can be cancelled, a finished one can be regenerated, saved,
// and downloaded.
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
	done.Status = view.ClipStatusCompleted
	done.FileExists = true

	var finished strings.Builder

	require.NoError(t, ClipStatus(done).Render(t.Context(), &finished))

	finishedBody := finished.String()
	assert.NotContains(t, finishedBody, "Cancel", "a finished clip cannot be cancelled")
	assert.Contains(t, finishedBody, "Regenerate")
	assert.Contains(t, finishedBody, "Save metadata")
	assert.Contains(t, finishedBody, "/api/clips/c1/download")
}

// TestClipStatusCarriesEverythingThatChanges pins the contents of the polled
// region. A clip that completes swaps the status badge, drops the progress bar,
// and gains the preview player, so all of it has to live inside the region or
// the card would not update until a manual reload.
func TestClipStatusCarriesEverythingThatChanges(t *testing.T) {
	t.Parallel()

	progressing := activeTestItem()
	progressing.Progress = 40
	progressing.Error = "boom"

	var active strings.Builder

	require.NoError(t, ClipStatus(progressing).Render(t.Context(), &active))

	activeBody := active.String()
	assert.Contains(t, activeBody, view.ClipStatusProcessing)
	assert.Contains(t, activeBody, "boom")
	assert.Contains(t, activeBody, "40%")
	assert.Contains(t, activeBody, `hx-get="/clips/c1/row"`)
	assert.NotContains(t, activeBody, "Preview", "no player while the clip is still encoding")

	done := activeTestItem()
	done.Status = view.ClipStatusCompleted
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

// activeTestItem is an encoding clip, which is the state that polls.
func activeTestItem() view.ClipItem {
	return view.ClipItem{
		ID:          "c1",
		Name:        "Intro",
		MediaID:     "42",
		MediaTitle:  "Movie",
		ClipType:    "clip",
		Status:      view.ClipStatusProcessing,
		Progress:    40,
		StartTime:   1,
		Duration:    5,
		Quality:     "archive",
		ProfileName: "Archive",
		Profiles: []view.ClipProfileOption{
			{ID: "archive", Name: "Archive"},
		},
		FileExists: false,
		MaxDur:     600,
	}
}

func TestClipCard(t *testing.T) {
	t.Parallel()

	base := view.ClipItem{
		ID:          "c1",
		Name:        "Intro",
		MediaID:     "42",
		MediaTitle:  "Movie",
		ClipType:    "clip",
		Status:      view.ClipStatusCompleted,
		Progress:    100,
		CreatedAt:   "2026-01-01T00:00:00Z",
		StartTime:   1,
		Duration:    5,
		Quality:     "archive",
		ProfileName: "Archive",
		Profiles: []view.ClipProfileOption{
			{ID: "archive", Name: "Archive", IsDefault: false},
		},
		FileExists:    true,
		AudioIndex:    0,
		AudioTracks:   nil,
		CropBlackBars: false,
		MaxDur:        600,
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
				item.Status = view.ClipStatusCancelled
				item.FileExists = false
			},
			contains: []string{view.ClipStatusCancelled},
			notContains: []string{
				"Missing file",
				"On disk",
				"hx-trigger",
			},
		},
		{
			name: "gif preview",
			tweak: func(item *view.ClipItem) {
				item.ClipType = "gif"
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
				item.ClipType = "screenshot"
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
				item.Status = view.ClipStatusProcessing
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
		Status:  view.ClipStatusCompleted,
		Type:    "gif",
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
