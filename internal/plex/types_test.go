// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsContainerType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mediaType string
		want      bool
	}{
		{name: "show is browsable", mediaType: TypeShow, want: true},
		{name: "season is browsable", mediaType: TypeSeason, want: true},
		{name: "artist is browsable", mediaType: TypeArtist, want: true},
		{name: "album is browsable", mediaType: TypeAlbum, want: true},
		{name: "episode is a leaf", mediaType: TypeEpisode, want: false},
		{name: "movie is a leaf", mediaType: "movie", want: false},
		{name: "track is a leaf", mediaType: "track", want: false},
		{name: "photo is a leaf", mediaType: "photo", want: false},
		{name: "clip is a leaf", mediaType: "clip", want: false},
		{name: "directory is a leaf", mediaType: "directory", want: false},
		{name: "empty type is a leaf", mediaType: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, IsContainerType(test.mediaType))
		})
	}
}

func TestEmptyServer(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Server{}, EmptyServer())
	assert.Empty(t, EmptyServer().Address)
	assert.Equal(t, 0, EmptyServer().Port)
	assert.False(t, EmptyServer().Local)
}
