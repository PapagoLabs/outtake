// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

// SessionsWhere keeps the playback sessions whose account passes a check.
//
// Parameters:
//   - sessions: Every session the server reported.
//   - includes: Reports whether the asking account may see a session played
//     by the given plex.tv account id.
//
// Returns:
//   - visible: The sessions the check keeps, in their order.
func SessionsWhere(sessions []Session, includes func(userID int) bool) []Session {
	visible := make([]Session, 0, len(sessions))

	for index := range sessions {
		if includes(sessions[index].UserID) {
			visible = append(visible, sessions[index])
		}
	}

	return visible
}
