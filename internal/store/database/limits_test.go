// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/quota"
)

// TestPendingUserWaitsUntilApproved covers an account that asks to use the
// installation.
//
// It reads as pending until approved, then as a member, and approving it
// again finds nothing to approve.
func TestPendingUserWaitsUntilApproved(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	require.NoError(t, db.AddPendingUser(t.Context(), 7, "friend"))
	require.NoError(
		t,
		db.AddPendingUser(t.Context(), 7, "friend"),
		"asking again keeps the one row",
	)

	role, found, err := db.UserRole(t.Context(), 7)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, RolePending, role)

	require.NoError(t, db.ApproveUser(t.Context(), 7))

	role, _, err = db.UserRole(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, RoleMember, role)

	require.ErrorIs(t, db.ApproveUser(t.Context(), 7), ErrUserNotFound)
}

// TestUserChangesNeverReachTheOwner covers the changes the users page makes.
//
// A member can be made an admin and back, given limits, and removed. The
// owner takes none of those changes.
func TestUserChangesNeverReachTheOwner(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	_, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	require.NoError(t, db.AddPendingUser(t.Context(), 7, "friend"))
	require.NoError(t, db.ApproveUser(t.Context(), 7))

	require.NoError(t, db.SetUserRole(t.Context(), 7, RoleAdmin))

	role, _, err := db.UserRole(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, role)

	require.ErrorIs(
		t,
		db.SetUserRole(t.Context(), 7, RoleOwner),
		ErrUserNotFound,
		"nobody is made the owner",
	)
	require.ErrorIs(t, db.SetUserRole(t.Context(), 42, RoleMember), ErrUserNotFound)
	require.ErrorIs(t, db.RemoveUser(t.Context(), 42, RoleOwner), ErrUserNotFound)
	require.ErrorIs(t, db.RemoveUser(t.Context(), 42, RoleAdmin), ErrUserNotFound,
		"the owner is never removed, whatever role is named")
	require.ErrorIs(t, db.SaveUserLimits(t.Context(), 42, quota.Override{}), ErrUserNotFound)
	require.ErrorIs(t, db.SaveUserLimits(t.Context(), 7, quota.Override{}), ErrUserNotFound,
		"an admin has no limits")
	require.ErrorIs(t, db.RemoveUser(t.Context(), 7, RoleMember), ErrUserNotFound,
		"a removal checked against a member leaves an admin alone")

	require.NoError(t, db.RemoveUser(t.Context(), 7, RoleAdmin))

	_, found, err := db.UserRole(t.Context(), 7)
	require.NoError(t, err)
	assert.False(t, found)
}

// TestListUsersPutsTheOwnerFirst covers the users page's list, with each
// user's role and own limits.
func TestListUsersPutsTheOwnerFirst(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	require.NoError(t, db.AddPendingUser(t.Context(), 7, "friend"))
	require.NoError(t, db.ApproveUser(t.Context(), 7))
	require.NoError(t, db.AddPendingUser(t.Context(), 8, "waiting"))

	_, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)

	active := 4
	require.NoError(t, db.SaveUserLimits(t.Context(), 7, quota.Override{Active: &active}))

	users, err := db.ListUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 3)

	assert.Equal(t, 42, users[0].PlexID)
	assert.Equal(t, RoleOwner, users[0].Role)
	assert.Equal(t, "friend", users[1].Username)
	assert.Equal(t, RoleMember, users[1].Role)
	assert.Equal(t, RolePending, users[2].Role)
	require.NotNil(t, users[1].Limits.Active)
	assert.Equal(t, 4, *users[1].Limits.Active)
	assert.Nil(t, users[1].Limits.Weekly, "a limit never set takes the global one")
	assert.False(t, users[1].LastLoginAt.IsZero())
}

// TestUserLimitsRoundTrip covers storing and reading one user's limits,
// including clearing one back to the global value.
func TestUserLimitsRoundTrip(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	require.NoError(t, db.AddPendingUser(t.Context(), 7, "friend"))
	require.ErrorIs(t, db.SaveUserLimits(t.Context(), 7, quota.Override{}), ErrUserNotFound,
		"an account waiting for approval has no limits of its own yet")
	require.NoError(t, db.ApproveUser(t.Context(), 7))

	weekly := 0
	storage := int64(5_000_000_000)
	clipLength := 90 * time.Second

	require.NoError(t, db.SaveUserLimits(t.Context(), 7, quota.Override{
		Weekly: &weekly, Storage: &storage, ClipLength: &clipLength,
	}))

	got, err := db.UserLimits(t.Context(), 7)
	require.NoError(t, err)
	assert.Nil(t, got.Active)
	assert.Equal(t, 0, *got.Weekly)
	assert.Equal(t, storage, *got.Storage)
	assert.Equal(t, clipLength, *got.ClipLength)

	require.NoError(t, db.SaveUserLimits(t.Context(), 7, quota.Override{}))

	got, err = db.UserLimits(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, quota.Override{}, got)

	unknown, err := db.UserLimits(t.Context(), 99)
	require.NoError(t, err)
	assert.Equal(t, quota.Override{}, unknown, "a user not stored has no limits of their own")
}

// TestGlobalLimitsDefaultUntilSaved covers the limits every member has.
func TestGlobalLimitsDefaultUntilSaved(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	limits, err := db.GlobalLimits(t.Context())
	require.NoError(t, err)
	assert.Equal(t, quota.Defaults(), limits)

	saved := quota.Limits{ClipLength: 2 * time.Minute, Storage: 1_000, Active: 0, Weekly: 7}
	require.NoError(t, db.SaveGlobalLimits(t.Context(), saved))

	limits, err = db.GlobalLimits(t.Context())
	require.NoError(t, err)
	assert.Equal(t, saved, limits)

	require.NoError(t, db.SaveSetting(t.Context(), settingLimitWeekly, "many"))

	_, err = db.GlobalLimits(t.Context())
	require.Error(t, err, "a stored limit that is not a number is reported")
}

// TestRendersSinceCountsTheWindow covers the weekly count, which counts only
// the user's renders since the start of the window.
func TestRendersSinceCountsTheWindow(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)
	now := time.Date(2026, time.October, 12, 12, 0, 0, 0, time.UTC)

	for _, at := range []time.Time{now.Add(-8 * 24 * time.Hour), now.Add(-2 * time.Hour), now.Add(-time.Hour)} {
		require.NoError(t, db.RecordRender(t.Context(), 7, at))
	}

	require.NoError(t, db.RecordRender(t.Context(), 42, now))

	started, oldest, err := db.RendersSince(t.Context(), 7, now.Add(-quota.Window))
	require.NoError(t, err)
	assert.Equal(t, 2, started)
	assert.True(t, now.Add(-2*time.Hour).Equal(oldest))

	started, oldest, err = db.RendersSince(t.Context(), 9, now.Add(-quota.Window))
	require.NoError(t, err)
	assert.Zero(t, started)
	assert.True(t, oldest.IsZero())
}

// TestStorageUsedSumsTheUsersClips covers the storage limit's measure, the
// bytes of the user's own clips.
func TestStorageUsedSumsTheUsersClips(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)
	stamp := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

	for index, bytes := range []int64{100, 250} {
		job := testStoredClip("mine-"+string(rune('a'+index)), "1", stamp)

		job.OwnerID = 7
		job.OutputBytes = bytes
		require.NoError(t, db.SaveClip(t.Context(), job))
	}

	theirs := testStoredClip("theirs", "1", stamp)

	theirs.OwnerID = 42
	theirs.OutputBytes = 999
	theirs.Status = clip.StatusCompleted
	require.NoError(t, db.SaveClip(t.Context(), theirs))

	used, err := db.StorageUsed(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, int64(350), used)

	none, err := db.StorageUsed(t.Context(), 9)
	require.NoError(t, err)
	assert.Zero(t, none)
}

// TestResetOwnerRemovesEveryUser covers handing the installation to another
// account, which takes its members and admins with it.
func TestResetOwnerRemovesEveryUser(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	_, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	require.NoError(t, db.AddPendingUser(t.Context(), 7, "friend"))

	removed, err := db.ResetOwner(t.Context())
	require.NoError(t, err)
	assert.True(t, removed)

	users, err := db.ListUsers(t.Context())
	require.NoError(t, err)
	assert.Empty(t, users)
}

// TestMigrationUserLimitsKeepsEveryUserApproved covers an installation that
// had users before accounts could wait for approval.
//
// Every stored user stays approved with no limits of their own, and older
// clips take no storage until they render again.
func TestMigrationUserLimitsKeepsEveryUserApproved(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/older-users.db"

	db, err := New(path)
	require.NoError(t, err)

	_, err = db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)

	stamp := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.SaveClip(t.Context(), testStoredClip("older", "1", stamp)))

	for _, statement := range []string{
		`ALTER TABLE users DROP COLUMN approved`,
		`ALTER TABLE users DROP COLUMN limit_active`,
		`ALTER TABLE users DROP COLUMN limit_weekly`,
		`ALTER TABLE users DROP COLUMN limit_storage_bytes`,
		`ALTER TABLE users DROP COLUMN limit_clip_seconds`,
		`ALTER TABLE clips DROP COLUMN output_bytes`,
		`DROP TABLE render_log`,
		`INSERT INTO users (plex_user_id, username, role) VALUES (7, 'friend', 'member')`,
		`DELETE FROM schema_migrations WHERE name = '014_user_limits.sql'`,
	} {
		_, err = db.Conn().ExecContext(t.Context(), statement)
		require.NoError(t, err, statement)
	}

	require.NoError(t, db.Close())

	db, err = New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	role, _, err := db.UserRole(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, RoleMember, role, "a user stored before stays approved")

	limits, err := db.UserLimits(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, quota.Override{}, limits)

	older, err := db.GetClip(t.Context(), "older")
	require.NoError(t, err)
	assert.Zero(t, older.OutputBytes)
}

// TestSweepRenderLogDropsOnlyRendersPastTheWindow covers the render log's
// cleanup, which keeps every render the weekly limit still counts.
func TestSweepRenderLogDropsOnlyRendersPastTheWindow(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)
	now := time.Date(2026, time.October, 12, 12, 0, 0, 0, time.UTC)

	require.NoError(t, db.RecordRender(t.Context(), 7, now.Add(-8*24*time.Hour)))
	require.NoError(t, db.RecordRender(t.Context(), 7, now.Add(-time.Hour)))

	removed, err := db.SweepRenderLog(t.Context(), now.Add(-quota.Window))
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)

	started, _, err := db.RendersSince(t.Context(), 7, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, started, "the render inside the window is kept")
}

// TestRejectUserLeavesAnApprovedAccountAlone covers a reject that arrives
// after the account was approved, which removes nothing.
func TestRejectUserLeavesAnApprovedAccountAlone(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	require.NoError(t, db.AddPendingUser(t.Context(), 5, "waiting"))
	require.NoError(t, db.AddPendingUser(t.Context(), 7, "approved"))
	require.NoError(t, db.ApproveUser(t.Context(), 7))

	require.NoError(t, db.RejectUser(t.Context(), 5))
	require.ErrorIs(t, db.RejectUser(t.Context(), 7), ErrUserNotFound)

	_, found, err := db.UserRole(t.Context(), 5)
	require.NoError(t, err)
	assert.False(t, found)

	role, _, err := db.UserRole(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, RoleMember, role)
}

// TestUsageByOwnerGroupsEveryUser covers the users page's usage, read for
// every user at once.
//
// Each user's renders inside the span and their clips' bytes are summed, and
// a user with neither is absent.
func TestUsageByOwnerGroupsEveryUser(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)
	now := time.Date(2026, time.October, 12, 12, 0, 0, 0, time.UTC)

	for _, render := range []struct {
		at    time.Time
		owner int
	}{
		{at: now.Add(-time.Hour), owner: 7},
		{at: now.Add(-2 * time.Hour), owner: 7},
		{at: now.Add(-8 * 24 * time.Hour), owner: 7},
		{at: now.Add(-time.Hour), owner: 9},
	} {
		require.NoError(t, db.RecordRender(t.Context(), render.owner, render.at))
	}

	for index, clipOwner := range []int{7, 7, 42} {
		job := testStoredClip("clip-"+string(rune('a'+index)), "1", now)

		job.OwnerID = clipOwner
		job.OutputBytes = 100
		require.NoError(t, db.SaveClip(t.Context(), job))
	}

	usage, err := db.UsageByOwner(t.Context(), now.Add(-quota.Window))
	require.NoError(t, err)

	assert.Equal(t, UserUsage{Bytes: 200, Renders: 2}, usage[7])
	assert.Equal(t, UserUsage{Bytes: 0, Renders: 1}, usage[9])
	assert.Equal(t, UserUsage{Bytes: 100, Renders: 0}, usage[42])
	assert.NotContains(t, usage, 5)
}

// TestSetUserRoleNeedsTheOtherRole covers a role change that finds the user
// already holding the role it was checked against no longer.
func TestSetUserRoleNeedsTheOtherRole(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	require.NoError(t, db.AddPendingUser(t.Context(), 7, "friend"))
	require.NoError(t, db.ApproveUser(t.Context(), 7))

	require.ErrorIs(t, db.SetUserRole(t.Context(), 7, RoleMember), ErrUserNotFound,
		"making a member a member finds no admin to change")
	require.NoError(t, db.SetUserRole(t.Context(), 7, RoleAdmin))
	require.ErrorIs(t, db.SetUserRole(t.Context(), 7, RoleAdmin), ErrUserNotFound,
		"making an admin an admin finds no member to change")
	require.NoError(t, db.SetUserRole(t.Context(), 7, RoleMember))
}

// TestUsernamesNamesEveryApprovedUser covers the names shown on another
// account's shared clip.
//
// An account waiting for approval is named only once it is approved.
func TestUsernamesNamesEveryApprovedUser(t *testing.T) {
	t.Parallel()

	db := usersDatabase(t)

	_, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	require.NoError(t, db.AddPendingUser(t.Context(), 7, "friend"))

	names, err := db.Usernames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[int]string{42: "nick"}, names)

	require.NoError(t, db.ApproveUser(t.Context(), 7))

	names, err = db.Usernames(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[int]string{42: "nick", 7: "friend"}, names)
}
