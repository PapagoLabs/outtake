// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDisplayName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Intro", DisplayName("Intro", "Movie"))
	assert.Equal(t, "Movie", DisplayName("", "Movie"),
		"an unnamed clip is labeled by what it was cut from")
	assert.Empty(t, DisplayName("", ""))
}

func TestFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give Clip
		want string
	}{
		{
			name: "a named clip keeps its name",
			give: Clip{ID: "c1", Name: "Intro", MediaTitle: "Movie", Type: TypeClip},
			want: "Intro.mp4",
		},
		{
			name: "an unnamed clip is named after its source",
			give: Clip{ID: "c1", MediaTitle: "Movie", Type: TypeClip},
			want: "Movie.mp4",
		},
		{
			name: "a clip with nothing to name it falls back to its id",
			give: Clip{ID: "c1", Type: TypeClip},
			want: "c1.mp4",
		},
		{
			name: "a GIF is delivered as a GIF",
			give: Clip{ID: "c1", Name: "Loop", Type: TypeGIF},
			want: "Loop.gif",
		},
		{
			name: "a screenshot is delivered as a JPEG",
			give: Clip{ID: "c1", Name: "Frame", Type: TypeScreenshot},
			want: "Frame.jpg",
		},
		{
			name: "an unrecognized type is delivered as a clip",
			give: Clip{ID: "c1", Name: "Odd", Type: "hologram"},
			want: "Odd.mp4",
		},
		{
			name: "a separator cannot escape the directory it is named in",
			give: Clip{ID: "c1", Name: "A/B\\C", Type: TypeClip},
			want: "A-B-C.mp4",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.give.Filename())
		})
	}
}
