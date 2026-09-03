// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestSelectedLibraryID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		giveCurrent string
		giveQuery   string
		want        string
	}{
		{giveCurrent: "", giveQuery: "7", want: "7"},
		{giveCurrent: "http://localhost:8080/media?library=3", giveQuery: "", want: "3"},
		{giveCurrent: "http://localhost:8080/clips", giveQuery: "", want: ""},
		{giveCurrent: "://bad", giveQuery: "", want: ""},
		{giveCurrent: "http://localhost:8080/media?library=9", giveQuery: "1", want: "1"},
	}

	for _, test := range tests {
		assert.Equal(t, test.want, selectedLibraryID(test.giveCurrent, test.giveQuery))
	}
}

func TestMediaItemError(t *testing.T) {
	t.Parallel()

	_ = t.Context()

	assert.Equal(t, "bad duration", mediaItemError(nil, "bad duration"))
	assert.Equal(t, "bad duration", mediaItemError(errNoPlexServer, "bad duration"))
	assert.Equal(t, mediaLoadFailedMsg, mediaItemError(errNoPlexServer, ""))
	assert.Empty(t, mediaItemError(nil, ""))
}

func TestClipProfileName(t *testing.T) {
	t.Parallel()

	profiles := []view.ClipProfileOption{
		{ID: "medium", Name: "Medium", IsDefault: true},
		{ID: "archive", Name: "Archive", IsDefault: false},
	}

	assert.Equal(t, "Archive", clipProfileName("archive", profiles))
	assert.Equal(t, "low", clipProfileName("low", profiles))
	assert.Empty(t, clipProfileName("", nil))
}

func TestClipFileExists(t *testing.T) {
	t.Parallel()

	assert.False(t, clipFileExists(""))
	assert.False(t, clipFileExists(filepath.Join(t.TempDir(), "missing.mp4")))

	path := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	assert.True(t, clipFileExists(path))
}

func TestAudioTrackLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"eng · dts · 7.1 · DTS:X 7.1",
		audioTrackLabel(media.AudioTrack{
			Index:    0,
			Codec:    "dts",
			Language: "eng",
			Title:    "DTS:X 7.1",
			Channels: 8,
		}),
	)
	assert.Equal(
		t,
		"Track 2",
		audioTrackLabel(media.AudioTrack{
			Index:    1,
			Codec:    "",
			Language: "",
			Title:    "",
			Channels: 0,
		}),
	)
}
