// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

// Viewer is the account a request reads the bound Plex server as.
//
// The owner reads it with the bound server's own token. Anyone else reads it
// with their own token for that server, so Plex's sharing decides what they
// see.
type Viewer struct {
	// Token is the viewer's own token for the bound server, empty for the
	// owner and for an account the server is not shared with.
	Token string
	// PlexID is the viewer's plex.tv account id.
	PlexID int
	// Owner is true for the installation's owner, who reads the server with
	// its bound token.
	Owner bool
}

// OwnerViewer is the viewer for the installation's owner.
//
// Parameters:
//   - plexID: The owner's plex.tv account id.
//
// Returns:
//   - viewer: A viewer reading with the bound server's token.
func OwnerViewer(plexID int) Viewer {
	return Viewer{Token: "", PlexID: plexID, Owner: true}
}
