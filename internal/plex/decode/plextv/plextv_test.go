// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plextv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevices(t *testing.T) {
	t.Parallel()

	body := []byte(`<MediaContainer>
		<Device name="Home" accessToken="tok">
			<Connection uri="https://plex.example.com" address="plex.example.com" port="443" protocol="https" local="0"/>
		</Device>
	</MediaContainer>`)
	devices, err := Devices(body)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "Home", devices[0].Name)
	assert.Equal(t, "tok", devices[0].AccessToken)
	require.Len(t, devices[0].Connection, 1)
	assert.Equal(t, "https://plex.example.com", devices[0].Connection[0].URI)
}

func TestDevices_NoConnections(t *testing.T) {
	t.Parallel()

	// testDeviceBody is a plex.tv /api/resources envelope with no connections.
	const testDeviceBody = `<MediaContainer>
		<Device name="Home" address="" port="0" accessToken="tok"/>
	</MediaContainer>`

	devices, err := Devices([]byte(testDeviceBody))
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "Home", devices[0].Name)
	assert.Empty(t, devices[0].Connection)
}

func TestDevices_Malformed(t *testing.T) {
	t.Parallel()

	devices, err := Devices([]byte(`<MediaContainer><Video></MediaContainer>`))
	require.Error(t, err)
	require.ErrorContains(t, err, "decode devices")
	assert.Nil(t, devices)
}

func TestSearch(t *testing.T) {
	t.Parallel()

	// testSearchBody is a plex.tv /search envelope with both hit kinds.
	const testSearchBody = `<MediaContainer size="3">
		<Video ratingKey="900" key="/library/metadata/900" title="Alpha Movie"
			duration="60000" thumb="/library/metadata/900/thumb/1" type="movie"/>
		<Directory key="/library/sections/1" title="Movies" type="show"/>
	</MediaContainer>`

	videos, directories, err := Search([]byte(testSearchBody))
	require.NoError(t, err)
	require.Len(t, videos, 1)
	assert.Equal(t, "900", videos[0].RatingKey)
	assert.Equal(t, "Alpha Movie", videos[0].Title)
	assert.Equal(t, int64(60000), videos[0].Duration)
	assert.Equal(t, "/library/metadata/900/thumb/1", videos[0].Thumb)
	assert.Equal(t, "movie", videos[0].Type)
	require.Len(t, directories, 1)
	assert.Equal(t, "/library/sections/1", directories[0].Key)
	assert.Equal(t, "Movies", directories[0].Title)
	assert.Equal(t, "show", directories[0].Type)
}

func TestSearch_EmptyEnvelope(t *testing.T) {
	t.Parallel()

	videos, directories, err := Search([]byte(`<MediaContainer size="0"/>`))
	require.NoError(t, err)
	assert.Empty(t, videos)
	assert.Empty(t, directories)
}

func TestSearch_Malformed(t *testing.T) {
	t.Parallel()

	videos, directories, err := Search([]byte(`<MediaContainer><Video></MediaContainer>`))
	require.Error(t, err)
	require.ErrorContains(t, err, "decode search")
	assert.Nil(t, videos)
	assert.Nil(t, directories)
}

func TestSessionsThumb(t *testing.T) {
	t.Parallel()

	body := []byte(`<MediaContainer>
		<Video title="Now Playing" duration="3600000" thumb="/library/metadata/1/thumb/2">
			<Session id="sess-123"/>
		</Video>
	</MediaContainer>`)
	sessions, err := Sessions(body)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "/library/metadata/1/thumb/2", sessions[0].Thumb)
	assert.Equal(t, "/library/metadata/1/thumb/2", sessions[0].Media().Thumb)
}

func TestSessions(t *testing.T) {
	t.Parallel()

	// testSessionBody is a plex.tv /status/sessions envelope.
	const testSessionBody = `<MediaContainer size="1">
		<Video ratingKey="42" key="/library/metadata/42" title="Now Playing"
			duration="3600000" viewOffset="120000" type="movie">
			<Session id="sess-123"/>
		</Video>
	</MediaContainer>`

	sessions, err := Sessions([]byte(testSessionBody))
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "sess-123", sessions[0].PlaybackID())
	assert.Equal(t, int64(120000), sessions[0].ViewOffset)

	media := sessions[0].Media()
	assert.Equal(t, "42", media.RatingKey)
	assert.Equal(t, "/library/metadata/42", media.Key)
	assert.Equal(t, "Now Playing", media.Title)
	assert.Equal(t, int64(3600000), media.Duration)
	assert.Equal(t, "movie", media.Type)
	assert.Equal(t, "42", media.ID())
}

func TestSessions_EmptyEnvelope(t *testing.T) {
	t.Parallel()

	sessions, err := Sessions([]byte(`<MediaContainer size="0"/>`))
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestSessions_Malformed(t *testing.T) {
	t.Parallel()

	sessions, err := Sessions([]byte(`<MediaContainer><Video></MediaContainer>`))
	require.Error(t, err)
	require.ErrorContains(t, err, "decode sessions")
	assert.Nil(t, sessions)
}

func TestPlaybackID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "sess-123", Session{Info: sessionInfo{ID: "sess-123"}}.PlaybackID())
	assert.Empty(t, Session{}.PlaybackID())
}

func TestMediaID(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "42", Media{
		RatingKey: "",
		Key:       "/library/metadata/42/children",
		Title:     "",
		Duration:  0,
		Thumb:     "",
		Type:      "",
	}.ID())
	assert.Empty(t, Media{
		RatingKey: "",
		Key:       "/hubs/metadata/42",
		Title:     "",
		Duration:  0,
		Thumb:     "",
		Type:      "",
	}.ID())
}

func TestMediaID_RatingKeyWins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry Media
		want  string
	}{
		{
			name:  "rating key is preferred",
			entry: Media{RatingKey: "7", Key: "/library/metadata/42/children"},
			want:  "7",
		},
		{
			name:  "key is used without a rating key",
			entry: Media{Key: "/library/metadata/42"},
			want:  "42",
		},
		{
			name:  "trailing slash is trimmed",
			entry: Media{Key: "/library/metadata/42/"},
			want:  "42",
		},
		{
			name:  "no identity at all",
			entry: Media{Key: ""},
			want:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.entry.ID())
		})
	}
}
