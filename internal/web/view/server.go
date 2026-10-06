// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"github.com/PapagoLabs/outtake/internal/plex"
)

// ServerItem is one discovered Plex server connection. It carries no token:
// the page posts Key, and the token is looked up again from Plex.
type ServerItem struct {
	Key      string
	Name     string
	Address  string
	Port     int
	Scheme   string
	Local    bool
	Relay    bool
	Selected bool
}

// ServerItems maps discovered servers onto the server picker models.
//
// Parameters:
//   - servers: Discovered servers.
//   - current: Currently bound server, used to mark the selection.
//
// Returns:
//   - items: One page model per discovered server, in the order they arrived.
func ServerItems(servers []plex.Server, current plex.Server) []ServerItem {
	items := make([]ServerItem, 0, len(servers))

	for _, server := range servers {
		items = append(items, ServerItem{
			Key:      plex.SelectionKey(server),
			Name:     server.Name,
			Address:  server.Address,
			Port:     server.Port,
			Scheme:   server.Scheme,
			Local:    server.Local,
			Relay:    server.Relay,
			Selected: plex.SameConnection(server, current),
		})
	}

	return items
}
