// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderBar renders a bulk bar.
//
// Parameters:
//   - t: The test that renders it.
//   - props: The bar.
//
// Returns:
//   - body: The rendered bar.
func renderBar(t *testing.T, props BulkBarProps) string {
	t.Helper()

	var buf strings.Builder

	require.NoError(t, BulkBar(props).Render(t.Context(), &buf))

	return buf.String()
}

// TestBulkBarActsOnTheListsSelectedCards covers the bar of a list with cards
// the viewer may change.
//
// Each action posts the list's selected cards and reports to the status line,
// Delete asks first, and the actions start hidden.
func TestBulkBarActsOnTheListsSelectedCards(t *testing.T) {
	t.Parallel()

	body := renderBar(t, BulkBarProps{
		ListID:     "clip-list",
		ToolbarID:  "clip-list-toolbar",
		StatusID:   "clip-bulk-status",
		Selectable: true,
	})

	assert.Contains(t, body, `data-bulk-list="clip-list"`)
	assert.Contains(t, body, `data-bulk-refresh="clip-list-toolbar"`)
	assert.Contains(t, body, `data-bulk-all`)
	assert.Contains(t, body, "Select all")
	assert.Contains(t, body, `data-bulk-actions hidden`)
	assert.Contains(t, body, `hx-include="#clip-list [data-bulk-select]:checked"`)
	assert.Contains(t, body, `hx-target="#clip-bulk-status"`)
	assert.Contains(t, body, `hx-vals="{&#34;action&#34;:&#34;delete&#34;}"`)
	assert.Contains(t, body, `hx-confirm="Delete the selected clips? This can&#39;t be undone."`)
	assert.NotContains(t, body, "Stop Sharing", "sharing waits for a second user")
	assert.NotContains(t, body, "Retry Failed", "nothing failed")
}

// TestBulkBarOffersSharingAndRetry covers the actions that depend on the
// installation and the list.
func TestBulkBarOffersSharingAndRetry(t *testing.T) {
	t.Parallel()

	body := renderBar(t, BulkBarProps{
		ListID:     "item-clip-list",
		StatusID:   "item-clip-bulk-status",
		MediaID:    "42",
		Failed:     2,
		Selectable: true,
		CanShare:   true,
	})

	assert.Contains(t, body, "Stop Sharing")
	assert.Contains(t, body, `hx-post="/api/clips/retry"`)
	assert.Contains(t, body, `hx-vals="{&#34;mediaId&#34;:&#34;42&#34;}"`)
	assert.Contains(t, body, "Retry Failed")
}

// TestBulkBarIsLeftOutWithNothingToActOn covers a list of read-only cards
// with nothing failed, which shows no bar.
func TestBulkBarIsLeftOutWithNothingToActOn(t *testing.T) {
	t.Parallel()

	assert.Empty(t, renderBar(t, BulkBarProps{ListID: "clip-list"}))
	assert.Contains(t, renderBar(t, BulkBarProps{ListID: "clip-list", Failed: 1}), "Retry Failed")
}

// TestClipCardSelectsOnlyAClipTheViewerMayChange covers the card's box for
// the bulk actions.
func TestClipCardSelectsOnlyAClipTheViewerMayChange(t *testing.T) {
	t.Parallel()

	item := finishedTestItem()

	body := renderCard(t, item)
	assert.Contains(t, body, `data-bulk-select`)
	assert.Contains(t, body, `name="id" value="c1"`)
	assert.Contains(t, body, `aria-label="Select Intro"`)

	item.ReadOnly = true

	assert.NotContains(t, renderCard(t, item), `data-bulk-select`)
}
