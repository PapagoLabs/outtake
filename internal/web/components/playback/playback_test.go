// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestPlaybackPanelMarkButtons(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := PlaybackPanel(view.Playback{
		Playing:    true,
		ViewOffset: 90.125,
		Title:      "",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Regexp(t, `js-mark-start[^>]*>Set start from Plex`, body)
	assert.Regexp(t, `js-mark-end[^>]*>Set end from Plex`, body)
}

// TestPlaybackPanelMarkOffsetKeepsMilliseconds guards the precision of the
// offsets handed to the mark handlers. A coarser value writes a mark the user
// never chose, and the handler then parses it straight into the form.
func TestPlaybackPanelMarkOffsetKeepsMilliseconds(t *testing.T) {
	t.Parallel()

	offsets := []float64{90.125, 3723.456, 0.007}

	for _, offset := range offsets {
		var buf strings.Builder

		err := PlaybackPanel(view.Playback{
			Playing:    true,
			ViewOffset: offset,
			Title:      "",
		}).Render(t.Context(), &buf)
		require.NoError(t, err)

		assert.Equal(
			t,
			2,
			strings.Count(buf.String(), `data-offset="`+media.FormatSeconds(offset)+`"`),
			"both mark buttons must carry the millisecond offset",
		)
	}
}
