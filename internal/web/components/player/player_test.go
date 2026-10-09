// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package player

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFrameWrapsThePlayer covers the frame: it marks itself for the scripts
// and holds whatever is rendered into it.
func TestFrameWrapsThePlayer(t *testing.T) {
	t.Parallel()

	var body strings.Builder

	video := templ.ComponentFunc(func(_ context.Context, writer io.Writer) error {
		_, err := io.WriteString(writer, "<video></video>")

		return err //nolint:wrapcheck // A test component passes the writer's error on.
	})

	require.NoError(t, Frame().Render(templ.WithChildren(t.Context(), video), &body))

	assert.Equal(t, `<div class="relative" data-player-frame><video></video></div>`, body.String())
}

// TestBadgeNamesWhatThePlayerPlays covers the badge: its label, the file it
// names for the scripts, and the hidden attribute a script clears, with any
// id and attributes a page adds.
func TestBadgeNamesWhatThePlayerPlays(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		props       BadgeProps
		contains    []string
		notContains []string
	}{
		{
			name:        "a shown badge",
			props:       BadgeProps{ID: "", Label: "HDR · 4K", Plays: "hdr", Hidden: false, Attributes: nil},
			contains:    []string{`data-format-badge="hdr">HDR · 4K</span>`, "absolute", "pointer-events-none"},
			notContains: []string{" hidden", "id="},
		},
		{
			name: "a hidden badge a script fills in",
			props: BadgeProps{
				ID:         "preview-format",
				Label:      "",
				Plays:      "preview",
				Hidden:     true,
				Attributes: templ.Attributes{"role": "status"},
			},
			contains:    []string{`<span id="preview-format"`, `data-format-badge="preview" hidden role="status"></span>`},
			notContains: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var body strings.Builder

			require.NoError(t, Badge(test.props).Render(t.Context(), &body))

			for _, want := range test.contains {
				assert.Contains(t, body.String(), want)
			}

			for _, unwanted := range test.notContains {
				assert.NotContains(t, body.String(), unwanted)
			}
		})
	}
}
