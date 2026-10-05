// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestPlaybackPanelMarkButtons(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := PlaybackPanel(view.Playback{
		Playing:    true,
		ViewOffset: 90125 * time.Millisecond,
		Title:      "",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Regexp(t, `js-mark-start[^>]*>Set start from Plex`, body)
	assert.Regexp(t, `js-mark-end[^>]*>Set end from Plex`, body)
}

func TestPlaybackPanelMarkOffsetKeepsMilliseconds(t *testing.T) {
	t.Parallel()

	offsets := []time.Duration{
		90125 * time.Millisecond,
		3723*time.Second + 456*time.Millisecond,
		7 * time.Millisecond,
	}

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
			strings.Count(
				buf.String(),
				`data-offset="`+timecode.FromDuration(offset).FormatSeconds()+`"`,
			),
			"both mark buttons must carry the millisecond offset",
		)
	}
}
