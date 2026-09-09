// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex/decode/plextv"
)


func TestServersFromDevices_AccessTokenAndURI(t *testing.T) {
	t.Parallel()

	devices := []plextv.Device{
		{
			Name:        "Home",
			Address:     "",
			Port:        0,
			AccessToken: "camel-token",
			Connection: []plextv.Connection{
				{
					URI:      "https://plex.example.com",
					Address:  "plex.example.com",
					Port:     443,
					Protocol: "https",
					Local:    0,
				},
				{
					URI:      "https://192.168.120.103:32400",
					Address:  "192.168.120.103",
					Port:     32400,
					Protocol: "https",
					Local:    1,
				},
			},
		},
	}

	got := serversFromDevices(devices)
	require.Len(t, got, 2)
	assert.Equal(t, "plex.example.com", got[0].Address)
	assert.Equal(t, 443, got[0].Port)
	assert.Equal(t, "https", got[0].Scheme)
	assert.Equal(t, "camel-token", got[0].Token)
	assert.False(t, got[0].Local)
	assert.Equal(t, "192.168.120.103", got[1].Address)
	assert.True(t, got[1].Local)
}

