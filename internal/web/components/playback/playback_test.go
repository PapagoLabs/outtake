// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"strings"
	"testing"

	viewplayback "github.com/PapagoLabs/outtake/internal/web/view/playback"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaybackPanelMarkButtons(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := PlaybackPanel(viewplayback.Playback{
		Playing:    true,
		ViewOffset: 90,
		Title:      "",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Regexp(t, `js-mark-start[^>]*>Set start from Plex`, body)
	assert.Regexp(t, `js-mark-end[^>]*>Set end from Plex`, body)
}
