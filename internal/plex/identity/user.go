// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

// Role is what a user may do in this installation.
type Role string

// User is a Plex account this installation recognizes.
type User struct {
	// PlexID is the Plex account id behind the user.
	PlexID int

	// Username is the Plex username, or the account title when Plex reports no
	// username.
	Username string

	// Role is what the user may do.
	Role Role
}

const (
	// RoleOwner is the Plex account the installation belongs to. There is at
	// most one.
	RoleOwner Role = "owner"

	// RoleAdmin may manage the installation on the owner's behalf.
	RoleAdmin Role = "admin"

	// RoleMember may use the installation without managing it.
	RoleMember Role = "member"
)
