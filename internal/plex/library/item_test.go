// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/plex"
)

func TestItemResponses(t *testing.T) {
	t.Parallel()

	got := ItemResponses([]plex.MediaItem{
		{
			ID:               "1",
			Title:            "The Movie",
			Type:             "movie",
			Duration:         5400,
			ThumbPath:        "/thumb/1",
			LibraryTitle:     "Movies",
			Year:             1995,
			TitleSort:        "Movie, The",
			GrandparentTitle: "",
		},
		{
			ID:               "2",
			Title:            "Pilot",
			Type:             plex.TypeEpisode,
			ParentIndex:      2,
			Index:            5,
			GrandparentTitle: "The Show",
		},
	})

	assert.Equal(t, []api.MediaItemResponse{
		{
			ID:           "1",
			Title:        "The Movie (1995)",
			Type:         "movie",
			Duration:     5400,
			ThumbPath:    "/thumb/1",
			LibraryTitle: "Movies",
			Year:         1995,
		},
		{
			ID:        "2",
			Title:     "The Show · S02E05 · Pilot",
			Type:      plex.TypeEpisode,
			Season:    2,
			Episode:   5,
			ShowTitle: "The Show",
		},
	}, got)
}

func TestItemResponsesEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, ItemResponses(nil))
}

func TestSessionResponses(t *testing.T) {
	t.Parallel()

	got := SessionResponses([]plex.Session{
		{
			ID:         "7",
			Duration:   600,
			ViewOffset: 120,
			MediaItem:  plex.MediaItem{ID: "1", Title: "The Movie", Type: "movie"},
		},
	})

	assert.Equal(t, []api.SessionResponse{
		{
			ID:         "7",
			MediaID:    "1",
			Title:      "The Movie",
			Duration:   600,
			ViewOffset: 120,
		},
	}, got)
}

func TestSessionResponsesEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, SessionResponses(nil))
}
