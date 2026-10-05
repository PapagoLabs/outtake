// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

func TestServerItemsMapsEveryField(t *testing.T) {
	t.Parallel()

	items := ServerItems([]plex.Server{
		{
			Name:    "local",
			Address: "10.0.0.5",
			Port:    32400,
			Scheme:  "http",
			Token:   "tok-a",
			Local:   true,
		},
		{
			Name:    "remote",
			Address: "10.0.0.9",
			Port:    443,
			Scheme:  "https",
			Token:   "tok-b",
		},
	}, plex.Server{})

	require.Len(t, items, 2)
	assert.Equal(t, ServerItem{
		Name:     "local",
		Address:  "10.0.0.5",
		Port:     32400,
		Scheme:   "http",
		Token:    "tok-a",
		Local:    true,
		Selected: false,
	}, items[0])
	assert.Equal(t, ServerItem{
		Name:    "remote",
		Address: "10.0.0.9",
		Port:    443,
		Scheme:  "https",
		Token:   "tok-b",
	}, items[1])
}

func TestServerItemsMarksTheBoundConnection(t *testing.T) {
	t.Parallel()

	current := plex.Server{
		Name:    "Bound",
		Address: "10.0.0.9",
		Port:    443,
		Scheme:  "https",
		Token:   "tok-c",
	}

	items := ServerItems([]plex.Server{
		{
			Name:    "local",
			Address: "10.0.0.5",
			Port:    32400,
			Scheme:  "http",
		},
		{
			Name:    "Same origin, other token",
			Address: current.Address,
			Port:    current.Port,
			Scheme:  current.Scheme,
		},
		{
			Name:    "Same host, other port",
			Address: current.Address,
			Port:    32401,
			Scheme:  current.Scheme,
		},
		{
			Name:    "Same host and port, other scheme",
			Address: current.Address,
			Port:    current.Port,
			Scheme:  "http",
		},
		{
			Name:    "Other scheme, other port",
			Address: current.Address,
			Port:    32401,
			Scheme:  "http",
		},
	}, current)

	require.Len(t, items, 5)
	assert.False(t, items[0].Selected)
	assert.True(t, items[1].Selected,
		"the row marked in use follows the origin, not the token Plex handed out")
	assert.False(t, items[2].Selected)
	assert.False(t, items[3].Selected)
	assert.False(t, items[4].Selected)
}

func TestServerItemsAreEmptyForNoServers(t *testing.T) {
	t.Parallel()

	assert.Empty(t, ServerItems(nil, plex.Server{}))
}
