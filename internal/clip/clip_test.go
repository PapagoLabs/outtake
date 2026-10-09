// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseType(t *testing.T) {
	t.Parallel()

	for _, give := range []string{"clip", "gif", "screenshot"} {
		got, ok := ParseType(give)
		require.True(t, ok, give)
		assert.Equal(t, Type(give), got)
	}

	for _, give := range []string{"nope", "video", ""} {
		_, ok := ParseType(give)
		assert.False(t, ok, give)
	}
}

func TestResolveType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		fallback Type
		want     Type
		wantErr  bool
	}{
		{
			name:     "a request that names no type keeps the stored one",
			raw:      "",
			fallback: TypeGIF,
			want:     TypeGIF,
		},
		{
			name:     "a request that names a type wins over the stored one",
			raw:      "screenshot",
			fallback: TypeGIF,
			want:     TypeScreenshot,
		},
		{
			name:     "a type this app does not produce is rejected",
			raw:      "hologram",
			fallback: TypeClip,
			wantErr:  true,
		},
		{
			name:     "the video alias this API still accepts is rejected",
			raw:      "video",
			fallback: TypeClip,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolveType(test.raw, test.fallback)

			if test.wantErr {
				require.ErrorIs(t, err, ErrUnknownType)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestClipApplyDefaults(t *testing.T) {
	t.Parallel()

	empty := &Clip{}
	empty.ApplyDefaults()
	assert.Equal(t, TypeClip, empty.Type)
	assert.Empty(t, empty.Quality, "an empty quality names the default profile")
	assert.Equal(t, DefaultMediaType, empty.MediaType)

	set := &Clip{Type: TypeGIF, Quality: "archive", MediaType: "show"}
	set.ApplyDefaults()
	assert.Equal(t, TypeGIF, set.Type)
	assert.Equal(t, "archive", set.Quality)
	assert.Equal(t, "show", set.MediaType)
}

func TestClipCloneIsIndependent(t *testing.T) {
	t.Parallel()

	original := &Clip{ID: "c1", Name: "Intro", Duration: 5}

	copied := original.Clone()

	copied.Name = "Outro"

	assert.Equal(t, "Intro", original.Name, "a clone must not share state with its original")
	assert.Nil(t, (*Clip)(nil).Clone(), "cloning nothing yields nothing")
}
