// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testMovie = "Movie"
	testShow  = "Show"
)

func TestDisplayTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give MediaItem
		want string
	}{
		{
			name: "movie with year",
			give: MediaItem{Title: testMovie, Type: "movie", Year: 1995},
			want: testMovie + " (1995)",
		},
		{
			name: "movie without year",
			give: MediaItem{Title: testMovie, Type: "movie"},
			want: testMovie,
		},
		{
			name: "episode with show and codes",
			give: MediaItem{
				Title:            "Episode 3",
				Type:             TypeEpisode,
				Index:            3,
				ParentIndex:      2,
				GrandparentTitle: testShow,
			},
			want: testShow + " · S02E03 · Episode 3",
		},
		{
			name: "episode title only",
			give: MediaItem{Title: "Episode", Type: TypeEpisode},
			want: "Episode",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.give.DisplayTitle())
		})
	}
}

func TestEpisodeCode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "S01E03", EpisodeCode(1, 3))
	assert.Equal(t, "E03", EpisodeCode(0, 3))
	assert.Equal(t, "S02", EpisodeCode(2, 0))
	assert.Empty(t, EpisodeCode(0, 0))
}

func TestSameConnection(t *testing.T) {
	t.Parallel()

	left := Server{Scheme: defaultScheme, Address: "plex.example", Port: 443}
	right := left
	assert.True(t, SameConnection(left, right))

	right.Port = 32400
	assert.False(t, SameConnection(left, right))
}
