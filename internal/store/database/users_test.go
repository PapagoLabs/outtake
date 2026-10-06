// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// usersDatabase opens a migrated SQLite database for a users test.
//
// Parameters:
//   - t: The test that owns the database.
//
// Returns:
//   - db: A database handle closed when the test finishes.
func usersDatabase(t *testing.T) *DB {
	t.Helper()

	db, err := New(filepath.Join(t.TempDir(), "users.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return db
}

func TestUserRoleReportsAnUnknownAccount(t *testing.T) {
	t.Parallel()

	role, found, err := usersDatabase(t).UserRole(t.Context(), 42)

	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, role)
}

func TestClaimOwnerStoresTheFirstAccount(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	owned, err := db.HasOwner(t.Context())
	require.NoError(t, err)
	assert.False(t, owned, "a fresh installation has no owner")

	claimed, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	assert.True(t, claimed)

	owned, err = db.HasOwner(t.Context())
	require.NoError(t, err)
	assert.True(t, owned)

	role, found, err := db.UserRole(t.Context(), 42)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "owner", role)
}

func TestClaimOwnerRefusesASecondAccount(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	claimed, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	require.True(t, claimed)

	claimed, err = db.ClaimOwner(t.Context(), 7, "intruder")
	require.NoError(t, err)
	assert.False(t, claimed, "the installation already has an owner")

	_, found, err := db.UserRole(t.Context(), 7)
	require.NoError(t, err)
	assert.False(t, found, "the refused account was not stored")
}

func TestClaimOwnerDoesNotClaimTwiceForTheSameAccount(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	claimed, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	require.True(t, claimed)

	claimed, err = db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	assert.False(t, claimed, "the second claim stored nothing")
}

func TestClaimOwnerLetsExactlyOneConcurrentClaimWin(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	const claimants = 16

	var (
		wins  atomic.Int32
		group sync.WaitGroup
	)

	var errs [claimants]error

	for claimant := range claimants {
		group.Go(func() {
			claimed, err := db.ClaimOwner(t.Context(), 100+claimant, "claimant")

			errs[claimant] = err

			if claimed {
				wins.Add(1)
			}
		})
	}

	group.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), wins.Load())
}

func TestTouchLoginRefreshesTheUsername(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	claimed, err := db.ClaimOwner(t.Context(), 42, "old-name")
	require.NoError(t, err)
	require.True(t, claimed)

	require.NoError(t, db.TouchLogin(t.Context(), 42, "new-name"))

	var username string

	err = db.conn.QueryRowContext(
		t.Context(),
		`SELECT username FROM users WHERE plex_user_id = ?`,
		42,
	).Scan(&username)
	require.NoError(t, err)
	assert.Equal(t, "new-name", username)
}

func TestTouchLoginIgnoresAnUnknownAccount(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	require.NoError(t, db.TouchLogin(t.Context(), 42, "nobody"))

	_, found, err := db.UserRole(t.Context(), 42)
	require.NoError(t, err)
	assert.False(t, found, "touching an unknown account does not create it")
}

func TestResetOwnerForgetsTheOwnerTheServerAndTheLegacyTokens(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	claimed, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	require.True(t, claimed)

	seedLegacyToken(t, db, "client", "legacy-token")
	require.NoError(t, db.SaveSelectedServer(t.Context(), plex.Server{
		Name:    "Attic",
		Address: "192.168.1.9",
		Port:    32400,
		Scheme:  "http",
		Token:   "server-token",
		Local:   true,
	}))

	removed, err := db.ResetOwner(t.Context())
	require.NoError(t, err)
	assert.True(t, removed)

	owned, err := db.HasOwner(t.Context())
	require.NoError(t, err)
	assert.False(t, owned)

	_, bound, err := db.SelectedServer(t.Context())
	require.NoError(t, err)
	assert.False(t, bound, "the owner's server went with the owner")

	token, err := db.LegacyToken(t.Context())
	require.NoError(t, err)
	assert.Empty(t, token, "the stored token was deleted")

	claimed, err = db.ClaimOwner(t.Context(), 7, "next")
	require.NoError(t, err)
	assert.True(t, claimed, "the next account claims the installation")
}

func TestResetOwnerReportsThatThereWasNoOwner(t *testing.T) {
	t.Parallel()

	removed, err := usersDatabase(t).ResetOwner(t.Context())

	require.NoError(t, err)
	assert.False(t, removed)
}

func TestSettingReportsAMissingValue(t *testing.T) {
	t.Parallel()

	value, found, err := usersDatabase(t).Setting(t.Context(), "absent")

	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
}

func TestSaveSettingStoresThenReplacesAValue(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	require.NoError(t, db.SaveSetting(t.Context(), "plex_client_id", "first"))
	require.NoError(t, db.SaveSetting(t.Context(), "plex_client_id", "second"))

	value, found, err := db.Setting(t.Context(), "plex_client_id")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "second", value)
}

func TestUsersReportAClosedDatabase(t *testing.T) {
	t.Parallel()

	db, err := New(filepath.Join(t.TempDir(), "closed.db"))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, _, err = db.UserRole(t.Context(), 42)
	require.ErrorContains(t, err, "user role")

	_, err = db.HasOwner(t.Context())
	require.ErrorContains(t, err, "has owner")

	_, err = db.ClaimOwner(t.Context(), 42, "nick")
	require.ErrorContains(t, err, "claim owner")

	err = db.TouchLogin(t.Context(), 42, "nick")
	require.ErrorContains(t, err, "touch login")

	_, err = db.ResetOwner(t.Context())
	require.ErrorContains(t, err, "reset owner")

	_, _, err = db.Setting(t.Context(), "name")
	require.ErrorContains(t, err, "setting name")

	err = db.SaveSetting(t.Context(), "name", "value")
	require.ErrorContains(t, err, "save setting name")

	_, err = db.LegacyToken(t.Context())
	require.ErrorContains(t, err, "legacy token")

	err = db.ClearLegacyTokens(t.Context())
	require.ErrorContains(t, err, "clear legacy tokens")

	err = db.ClearSelectedServer(t.Context())
	require.ErrorContains(t, err, "clear selected server")
}
