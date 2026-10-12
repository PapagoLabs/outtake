// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSessionsWhereKeepsWhatTheCheckAllows covers filtering sessions by the
// account playing them, in their order.
func TestSessionsWhereKeepsWhatTheCheckAllows(t *testing.T) {
	t.Parallel()

	sessions := []Session{
		{ID: "owner", UserID: 1},
		{ID: "member", UserID: 7},
		{ID: "again", UserID: 7},
	}

	mine := SessionsWhere(sessions, func(userID int) bool { return userID == 7 })

	assert.Len(t, mine, 2)
	assert.Equal(t, "member", mine[0].ID)
	assert.Equal(t, "again", mine[1].ID)
	assert.Empty(t, SessionsWhere(sessions, func(int) bool { return false }))
}
