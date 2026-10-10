// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func renderServers(t *testing.T, servers ...view.ServerItem) string {
	t.Helper()

	var buf strings.Builder

	err := Servers(ServersProps{Servers: servers}).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

func atticServer() view.ServerItem {
	return view.ServerItem{
		Name:     "Attic",
		Address:  "10.0.0.2",
		Port:     32400,
		Scheme:   "http",
		Key:      "attic-id http://10.0.0.2:32400",
		Local:    true,
		Selected: true,
	}
}

func basementServer() view.ServerItem {
	return view.ServerItem{
		Name:    "Basement",
		Address: "10.0.0.3",
		Port:    32400,
		Scheme:  "http",
		Key:     "basement-id http://10.0.0.3:32400",
	}
}

func TestServersMarksTheServerInUse(t *testing.T) {
	t.Parallel()

	body := renderServers(t, atticServer(), basementServer())

	assert.Contains(t, body, "bg-muted")
	assert.Contains(t, body, "ring-2")
	assert.Contains(t, body, "ring-sidebar-primary")
	assert.Contains(t, body, `aria-current="true"`)
	assert.Contains(t, body, ">In Use</span>")

	assert.Equal(t, 1, strings.Count(body, "ring-sidebar-primary"),
		"only the server in use is ringed in the sidebar accent")
	assert.Equal(t, 1, strings.Count(body, "bg-muted"),
		"only the server in use gets the muted fill")
	assert.Equal(t, 1, strings.Count(body, `aria-current="true"`),
		"only the server in use is marked as current")
	assert.NotContains(t, body, ">Current<",
		"the low contrast Current micro-label is gone")
}

func TestServersActiveRowOffersNoAction(t *testing.T) {
	t.Parallel()

	body := renderServers(t, atticServer(), basementServer())

	assert.Contains(t, body, "disabled>In Use</button>",
		"the server in use explains why it cannot be chosen")
	assert.Contains(t, body, `type="submit">Use This Server</button>`,
		"a server that is not in use stays selectable")
	assert.Equal(t, 1, strings.Count(body, "Use This Server"),
		"only the server that is not in use offers the switch action")
}

func TestServersKeepsEveryRowSubmittable(t *testing.T) {
	t.Parallel()

	body := renderServers(t, atticServer(), basementServer())

	assert.Equal(t, 3, strings.Count(body, `<form action="/servers" method="POST"`),
		"both rows and the custom URL section keep their own form")

	assert.Equal(t, 2, strings.Count(body, `name="server"`), "each row posts its selection key")
	assert.Contains(t, body, `value="attic-id http://10.0.0.2:32400"`)
	assert.Contains(t, body, `value="basement-id http://10.0.0.3:32400"`)

	for _, field := range []string{
		`name="name"`, `name="address"`, `name="port"`, `name="scheme"`, `name="token"`,
	} {
		assert.NotContains(t, body, field, "the page posts no %s", field)
	}
}

func TestServersKeepsCustomURLSection(t *testing.T) {
	t.Parallel()

	body := renderServers(t, atticServer())

	assert.Contains(t, body, `name="customUrl"`)
	assert.Contains(t, body, "Use This URL")
}

func TestServersEmptyState(t *testing.T) {
	t.Parallel()

	body := renderServers(t)

	assert.Contains(t, body, "No Plex servers found for this account")
	assert.NotContains(t, body, "ring-sidebar-primary")
	assert.NotContains(t, body, `aria-current="true"`)
}

func TestServersShowsTheSelectionError(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Servers(ServersProps{
		Servers: []view.ServerItem{atticServer()},
		Error:   "That server did not answer.",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "That server did not answer.")
	assert.Contains(t, body, "js-flash")
	assert.Contains(t, body, "Attic", "the discovered servers are still listed")
}

func TestServersMarksARemoteServer(t *testing.T) {
	t.Parallel()

	body := renderServers(t, atticServer(), basementServer())

	assert.Contains(t, body, "<span>(local)</span>")
	assert.Contains(t, body, "<span>(remote)</span>")
}
