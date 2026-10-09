// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFormatLabelNamesRangeAndResolution covers a player's badge: the range,
// then the resolution named as the export resolutions are when the width or
// the height is within 2% of one, so a trimmed 4K clip and a 4:3 clip keep
// their names, or the size written out for any other size, and nothing for an
// unread file.
func TestFormatLabelNamesRangeAndResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format Format
		want   string
	}{
		{name: "4K HDR", format: Format{Width: 3840, Height: 2160, HDR: true}, want: "HDR · 4K"},
		{
			name:   "letterboxed 4K",
			format: Format{Width: 3840, Height: 1608, HDR: true},
			want:   "HDR · 4K",
		},
		{
			name:   "4K with its black bars trimmed",
			format: Format{Width: 3838, Height: 1636, HDR: true},
			want:   "HDR · 4K",
		},
		{name: "1080p SDR", format: Format{Width: 1920, Height: 804}, want: "SDR · 1080p"},
		{name: "4:3 at 1080p", format: Format{Width: 1440, Height: 1080}, want: "SDR · 1080p"},
		{name: "720p SDR", format: Format{Width: 1280, Height: 720}, want: "SDR · 720p"},
		{name: "an unnamed size", format: Format{Width: 720, Height: 480}, want: "SDR · 720×480"},
		{
			name:   "a size beyond the tolerance",
			format: Format{Width: 2048, Height: 858},
			want:   "SDR · 2048×858",
		},
		{name: "never read", format: Format{}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.format.Label())
		})
	}
}
