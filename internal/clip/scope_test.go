// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestScopeIncludes covers which records a caller reaches.
//
// A member reaches only their own, the owner and admins reach every record,
// and a scope without a caller reaches nothing, not even a record nobody owns.
func TestScopeIncludes(t *testing.T) {
	t.Parallel()

	member := Scope{Owner: 7, Everyone: false}
	manager := Scope{Owner: 42, Everyone: true}
	nobody := Scope{Owner: 0, Everyone: false}

	assert.True(t, member.Includes(7))
	assert.False(t, member.Includes(42), "another account's record")
	assert.False(t, member.Includes(0), "a record nobody owns")
	assert.True(t, manager.Includes(7))
	assert.True(t, manager.Includes(0))
	assert.False(t, nobody.Includes(0))
	assert.False(t, nobody.Includes(7))
}

// TestScopeFilter covers keeping the jobs a caller reaches, in order.
func TestScopeFilter(t *testing.T) {
	t.Parallel()

	jobs := []*Job{
		{ID: "a", OwnerID: 7},
		{ID: "b", OwnerID: 42},
		{ID: "c", OwnerID: 7},
	}

	mine := Scope{Owner: 7, Everyone: false}.Filter(jobs)

	assert.Len(t, mine, 2)
	assert.Equal(t, "a", mine[0].ID)
	assert.Equal(t, "c", mine[1].ID)
	assert.Len(t, Scope{Owner: 42, Everyone: true}.Filter(jobs), 3)
}
