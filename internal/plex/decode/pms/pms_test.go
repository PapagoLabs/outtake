// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pms

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeLibraries(t *testing.T) {
	t.Parallel()

	body := []byte(`{"MediaContainer":{"Directory":[{"key":"1","title":"Movies","type":"movie"}]}}`)
	container, err := Decode(body)
	require.NoError(t, err)
	require.Len(t, container.Directory, 1)
	assert.Equal(t, "1", container.Directory[0].Key)
	assert.Equal(t, "Movies", container.Directory[0].Title)
}

func TestMetadataIDFromRatingKey(t *testing.T) {
	t.Parallel()

	body := []byte(
		`{"MediaContainer":{"Metadata":[{"ratingKey":200,"key":"/library/metadata/200/children","title":"A Show"}]}}`,
	)
	container, err := Decode(body)
	require.NoError(t, err)
	require.Len(t, container.Metadata, 1)
	assert.Equal(t, "200", container.Metadata[0].ID())
	assert.True(t, container.Metadata[0].HasMetadata())
}

func TestMetadataIDFromKeyPrefix(t *testing.T) {
	t.Parallel()

	meta := Metadata{
		RatingKey:  "",
		Key:        "/library/metadata/42/children",
		Title:      "",
		Type:       "",
		Duration:   0,
		ViewOffset: 0,
		Thumb:      "",
		Media:      nil,
		Session:    session{ID: ""},
	}
	assert.Equal(t, "42", meta.ID())
}

func TestMetadataFile(t *testing.T) {
	t.Parallel()

	meta := Metadata{
		RatingKey:  "",
		Key:        "",
		Title:      "",
		Type:       "",
		Duration:   0,
		ViewOffset: 0,
		Thumb:      "",
		Media:      []media{{Part: []part{{File: "/data/movie.mkv"}}}},
		Session:    session{ID: ""},
	}
	assert.Equal(t, "/data/movie.mkv", meta.File())

	empty := Metadata{
		RatingKey:  "",
		Key:        "",
		Title:      "",
		Type:       "",
		Duration:   0,
		ViewOffset: 0,
		Thumb:      "",
		Media:      nil,
		Session:    session{ID: ""},
	}
	assert.Empty(t, empty.File())
}

func TestDecodeEpisodeMetadata(t *testing.T) {
	t.Parallel()

	body := []byte(`{"MediaContainer":{"size":1,"totalSize":12,"offset":0,"Metadata":[{
		"ratingKey":"148392",
		"title":"Episode 3",
		"type":"episode",
		"year":2016,
		"index":3,
		"parentIndex":2,
		"parentRatingKey":"148395",
		"parentTitle":"Season 2",
		"grandparentRatingKey":"148394",
		"grandparentTitle":"Better Call Saul",
		"librarySectionID":2
	}]}}`)
	container, err := Decode(body)
	require.NoError(t, err)
	require.Len(t, container.Metadata, 1)

	meta := container.Metadata[0]
	assert.Equal(t, 12, container.TotalSize)
	assert.Equal(t, 2016, meta.Year)
	assert.Equal(t, 3, meta.Index)
	assert.Equal(t, 2, meta.ParentIndex)
	assert.Equal(t, "148395", string(meta.ParentRatingKey))
	assert.Equal(t, "Better Call Saul", meta.GrandparentTitle)
	assert.Equal(t, "2", string(meta.LibrarySectionID))
}

func TestDecodeLibraryThumb(t *testing.T) {
	t.Parallel()

	body := []byte(
		`{"MediaContainer":{"Directory":[{"key":"5","title":"Movies","type":"movie","thumb":"/library/sections/5/composite/1"}]}}`,
	)
	container, err := Decode(body)
	require.NoError(t, err)
	require.Len(t, container.Directory, 1)
	assert.Equal(t, "/library/sections/5/composite/1", container.Directory[0].Thumb)
}

func TestHasMetadataRejectsHubsPath(t *testing.T) {
	t.Parallel()

	meta := Metadata{
		RatingKey:  "",
		Key:        "/hubs/metadata/42",
		Title:      "",
		Type:       "",
		Duration:   0,
		ViewOffset: 0,
		Thumb:      "",
		Media:      nil,
		Session:    session{ID: ""},
	}
	assert.False(t, meta.HasMetadata())
	assert.Empty(t, meta.ID())
}
