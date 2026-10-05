// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex/decode/pms"
)

func TestDecodePMS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       []byte
		wantErr    error
		wantErrHas string
	}{
		{
			name:    "empty body",
			body:    []byte{},
			wantErr: errEmptyBody,
		},
		{
			name:    "nil body",
			body:    nil,
			wantErr: errEmptyBody,
		},
		{
			name:       "truncated json",
			body:       []byte(`{"MediaContainer":`),
			wantErrHas: "pms:",
		},
		{
			name:       "xml instead of json",
			body:       []byte(`<MediaContainer/>`),
			wantErrHas: "pms:",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			container, err := decodePMS(test.body)
			require.Error(t, err)

			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			}

			if test.wantErrHas != "" {
				require.ErrorContains(t, err, test.wantErrHas)
			}

			assert.Equal(t, pms.Container{}, container)
		})
	}
}

func TestDecodePMS_DecodesContainer(t *testing.T) {
	t.Parallel()

	// testJSONBody is a populated PMS JSON envelope.
	const testJSONBody = `{"MediaContainer":{
	"machineIdentifier":"abc123","version":"1.40.0","size":1,"totalSize":2,"offset":0,
	"Directory":[{"key":"1","title":"Movies","type":"movie","thumb":"/t/1"}],
	"Metadata":[{"ratingKey":"100","title":"Test Movie","type":"movie","duration":7200000}]
}}`

	container, err := decodePMS([]byte(testJSONBody))
	require.NoError(t, err)
	assert.Equal(t, "abc123", container.MachineIdentifier)
	assert.Equal(t, "1.40.0", container.Version)
	assert.Equal(t, 2, container.TotalSize)
	require.Len(t, container.Directory, 1)
	assert.Equal(t, "Movies", container.Directory[0].Title)
	require.Len(t, container.Metadata, 1)
	assert.Equal(t, "Test Movie", container.Metadata[0].Title)
}

func TestMetadataItems_SkipsRowsWithoutIdentity(t *testing.T) {
	t.Parallel()

	byRatingKey := pms.Metadata{RatingKey: "100", Title: "Test Movie", Type: "movie"}
	byKey := pms.Metadata{Key: "/library/metadata/42/children", Title: "Pilot"}
	directoryRow := pms.Metadata{
		Key:   "/library/sections/1/title",
		Title: "Movies",
		Type:  "directory",
	}

	items := metadataItems(
		[]pms.Metadata{directoryRow, byRatingKey, byKey, directoryRow},
		"Movies",
	)

	require.Len(t, items, 2)
	assert.Equal(t, "100", items[0].ID)
	assert.Equal(t, "Test Movie", items[0].Title)
	assert.Equal(t, "42", items[1].ID)
	assert.Equal(t, "Pilot", items[1].Title)

	for _, item := range items {
		assert.Equal(t, "Movies", item.LibraryTitle)
	}
}

func TestMetadataItems_EmptyInput(t *testing.T) {
	t.Parallel()

	assert.Empty(t, metadataItems(nil, ""))
	assert.Empty(t, metadataItems([]pms.Metadata{{Key: "/library/sections/1/title"}}, ""))
}

func TestMetadataToItem(t *testing.T) {
	t.Parallel()

	meta := pms.Metadata{
		RatingKey:            "100",
		Title:                "Test Movie",
		Type:                 "movie",
		Duration:             7200000,
		Thumb:                "/library/metadata/100/thumb/1",
		Year:                 1999,
		Index:                3,
		ParentIndex:          2,
		ParentRatingKey:      "20",
		ParentTitle:          "Season 2",
		GrandparentRatingKey: "10",
		GrandparentTitle:     "The Show",
		LibrarySectionID:     "4",
		TitleSort:            "test movie",
		AddedAt:              1700000000,
	}

	item := metadataToItem(&meta, "TV Shows")
	assert.Equal(t, "100", item.ID)
	assert.Equal(t, "movie", item.Type)
	assert.InEpsilon(t, 7200.0, item.Duration, 0.01)
	assert.Equal(t, "/library/metadata/100/thumb/1", item.ThumbPath)
	assert.Equal(t, "TV Shows", item.LibraryTitle)
	assert.Equal(t, "4", item.LibraryID)
	assert.Equal(t, 1999, item.Year)
	assert.Equal(t, 3, item.Index)
	assert.Equal(t, 2, item.ParentIndex)
	assert.Equal(t, "20", item.ParentID)
	assert.Equal(t, "Season 2", item.ParentTitle)
	assert.Equal(t, "10", item.GrandparentID)
	assert.Equal(t, "The Show", item.GrandparentTitle)
	assert.Equal(t, "test movie", item.TitleSort)
	assert.Equal(t, int64(1700000000), item.AddedAt)
}

func TestFirstMediaFile(t *testing.T) {
	t.Parallel()

	// testJSONFilesBody is a metadata envelope whose rows carry varying parts.
	const testJSONFilesBody = `{"MediaContainer":{"Metadata":[
	{"ratingKey":"100","Media":[{"Part":[
		{"file":"/media/first.mkv"},{"file":"/media/second.mkv"}
	]}]},
	{"ratingKey":"200","Media":[{"Part":[{}]}]},
	{"ratingKey":"300"}
]}}`

	container, err := pms.Decode([]byte(testJSONFilesBody))
	require.NoError(t, err)
	require.Len(t, container.Metadata, 3)

	assert.Equal(t, "/media/first.mkv", firstMediaFile(&container.Metadata[0]))
	assert.Empty(t, firstMediaFile(&container.Metadata[1]))
	assert.Empty(t, firstMediaFile(&container.Metadata[2]))
}
