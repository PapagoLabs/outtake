// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainclip "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// renderCard renders a clip card.
//
// Parameters:
//   - t: The test that renders it.
//   - item: The clip.
//
// Returns:
//   - body: The rendered card.
func renderCard(t *testing.T, item view.ClipItem) string {
	t.Helper()

	var buf strings.Builder

	require.NoError(t, ClipCard(item).Render(t.Context(), &buf))

	return buf.String()
}

// finishedTestItem is a finished clip whose file is stored.
//
// Returns:
//   - item: The clip.
func finishedTestItem() view.ClipItem {
	item := activeTestItem()

	item.Status = domainclip.StatusCompleted
	item.FileExists = true

	return item
}

// TestClipCardShowsAnotherAccountsClipReadOnly covers a shared clip on the
// card of a user who did not make it.
//
// It has no form and offers no change, only playback and Download, and it
// names its owner and that it is shared.
func TestClipCardShowsAnotherAccountsClipReadOnly(t *testing.T) {
	t.Parallel()

	item := finishedTestItem()

	item.ReadOnly = true
	item.Shared = true
	item.OwnerName = "<b>owner</b>"

	body := renderCard(t, item)

	assert.NotContains(t, body, "<form")
	assert.NotContains(t, body, "data-export-form")

	for _, change := range []string{"Delete", "Regenerate", "Save Changes", "Cancel", "/share"} {
		assert.NotContains(t, body, change)
	}

	assert.Contains(t, body, `href="/api/clips/c1/download"`)
	assert.Contains(t, body, `src="/clips/c1/file?v=`)
	assert.Contains(t, body, "By &lt;b&gt;owner&lt;/b&gt;", "the owner's name is escaped")
	assert.Contains(t, body, "Shared")

	rendering := activeTestItem()

	rendering.ReadOnly = true

	assert.NotContains(t, renderCard(t, rendering), "Cancel",
		"another account's render cannot be canceled")
}

// TestClipCardOffersSharingToWhoMayChangeIt covers the share button.
//
// A private clip offers Share and a shared one Stop Sharing, each posting to
// its route. A card that may not be shared offers neither.
func TestClipCardOffersSharingToWhoMayChangeIt(t *testing.T) {
	t.Parallel()

	item := finishedTestItem()

	item.CanShare = true

	private := renderCard(t, item)
	assert.Contains(t, private, `hx-post="/api/clips/c1/share"`)
	assert.Contains(t, private, "Share")
	assert.Contains(t, private, "<form", "a clip the viewer may change keeps its form")

	item.Shared = true

	shared := renderCard(t, item)
	assert.Contains(t, shared, `hx-post="/api/clips/c1/unshare"`)
	assert.Contains(t, shared, "Stop Sharing")

	item.CanShare = false

	assert.NotContains(t, renderCard(t, item), "/share")
}

// TestListToolbarOffersTheSharedClips covers the Show choice on the clips
// page.
//
// Its first option names the viewer's own list, every clip for the owner and
// admins, and the second the shared clips.
func TestListToolbarOffersTheSharedClips(t *testing.T) {
	t.Parallel()

	render := func(manages bool, props ListToolbarProps) string {
		var buf strings.Builder

		ctx := view.ContextWithManager(t.Context(), manages)
		require.NoError(t, ListToolbar(props).Render(ctx, &buf))

		return buf.String()
	}

	member := render(false, ListToolbarProps{OfferShared: true, Shared: true})
	assert.Contains(t, member, `name="show"`)
	assert.Contains(t, member, "My Clips")
	assert.Contains(t, member, `<option value="shared" selected>Shared Clips</option>`)

	assert.Contains(t, render(true, ListToolbarProps{OfferShared: true}), "All Clips")
	assert.NotContains(t, render(false, ListToolbarProps{}), `name="show"`,
		"a title's page lists its shared clips in their own section instead")
}
