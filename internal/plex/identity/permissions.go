// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

// Action is a change one user makes to another on the users page.
type Action string

// The changes a user can make to another.
const (
	// ActionApprove lets a pending account in as a member.
	ActionApprove Action = "approve"
	// ActionReject turns a pending account away.
	ActionReject Action = "reject"
	// ActionRemove deletes an approved user.
	ActionRemove Action = "remove"
	// ActionLimits sets a user's own render limits.
	ActionLimits Action = "limits"
	// ActionMakeAdmin makes a member an admin.
	ActionMakeAdmin Action = "make-admin"
	// ActionMakeMember makes an admin a member.
	ActionMakeMember Action = "make-member"
)

// May reports whether a user may make a change to another.
//
// Nobody changes the owner. The owner makes any change to anyone else. An
// admin approves and rejects pending accounts, and removes members and sets
// their limits, but leaves other admins alone and makes nobody an admin.
// Members and pending accounts change nobody. Only members have limits, so
// only theirs are set.
//
// Parameters:
//   - action: The change.
//   - target: The role of the user it is made to.
//
// Returns:
//   - allowed: True when the change is allowed.
func (user User) May(action Action, target Role) bool {
	if target == RoleOwner {
		return false
	}

	switch user.Role {
	case RoleOwner:
		return fits(action, target)
	case RoleAdmin:
		return adminMay(action, target)
	default:
		return false
	}
}

// adminMay reports whether an admin may make a change.
//
// Parameters:
//   - action: The change.
//   - target: The role of the user it is made to.
//
// Returns:
//   - allowed: True for approving or rejecting a pending account, and for
//     removing a member or setting its limits.
func adminMay(action Action, target Role) bool {
	switch action {
	case ActionApprove, ActionReject:
		return target == RolePending
	case ActionRemove, ActionLimits:
		return target == RoleMember
	default:
		return false
	}
}

// fits reports whether a change applies to a user of a role at all.
//
// Parameters:
//   - action: The change.
//   - target: The role of the user it is made to.
//
// Returns:
//   - applies: True when the change makes sense for the role.
func fits(action Action, target Role) bool {
	switch action {
	case ActionApprove, ActionReject:
		return target == RolePending
	case ActionRemove:
		return target == RoleMember || target == RoleAdmin
	case ActionLimits, ActionMakeAdmin:
		return target == RoleMember
	case ActionMakeMember:
		return target == RoleAdmin
	default:
		return false
	}
}
