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
