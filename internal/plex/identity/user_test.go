// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestUserManages covers which roles may manage the installation.
func TestUserManages(t *testing.T) {
	t.Parallel()

	for role, want := range map[Role]bool{
		RoleOwner:   true,
		RoleAdmin:   true,
		RoleMember:  false,
		RolePending: false,
		"":          false,
	} {
		assert.Equal(t, want, User{PlexID: 1, Username: "", Role: role}.Manages(), string(role))
	}
}
