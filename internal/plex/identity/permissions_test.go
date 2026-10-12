// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMayAllowsOnlyTheChangesEachRoleMakes covers every change between
// every pair of roles.
func TestMayAllowsOnlyTheChangesEachRoleMakes(t *testing.T) {
	t.Parallel()

	actions := []Action{
		ActionApprove, ActionReject, ActionRemove, ActionLimits, ActionMakeAdmin, ActionMakeMember,
	}

	allowed := map[Role]map[Role][]Action{
		RoleOwner: {
			RolePending: {ActionApprove, ActionReject},
			RoleMember:  {ActionRemove, ActionLimits, ActionMakeAdmin},
			RoleAdmin:   {ActionRemove, ActionMakeMember},
		},
		RoleAdmin: {
			RolePending: {ActionApprove, ActionReject},
			RoleMember:  {ActionRemove, ActionLimits},
		},
		RoleMember:  {},
		RolePending: {},
	}

	for _, actor := range []Role{RoleOwner, RoleAdmin, RoleMember, RolePending} {
		for _, target := range []Role{RoleOwner, RoleAdmin, RoleMember, RolePending} {
			for _, action := range actions {
				want := false

				for _, permitted := range allowed[actor][target] {
					if permitted == action {
						want = true
					}
				}

				got := User{PlexID: 1, Username: "", Role: actor}.May(action, target)
				assert.Equal(t, want, got, "%s %s %s", actor, action, target)
			}
		}
	}

	assert.False(t, User{PlexID: 1, Username: "", Role: RoleOwner}.May("unknown", RoleMember))
}
