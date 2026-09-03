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
