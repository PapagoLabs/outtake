// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package users

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/quota"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/middleware/middlewaretest"
	"github.com/PapagoLabs/outtake/internal/web/respond/respondtest"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// reply is what one request answered.
type reply struct {
	// body is the response's body.
	body string
	// location is the redirect target.
	location string
	// flash is the message carried to the next page.
	flash string
	// status is its status code.
	status int
}

// staleRoles reads users from a database but reports some roles as they
// were before a change another admin made.
type staleRoles struct {
	*database.DB

	// roles are the roles reported in place of the stored ones.
	roles map[int]string
}

// Accounts the tests sign in as.
const (
	// ownerID is the owner's plex.tv id.
	ownerID = 42
	// adminID is an admin's plex.tv id.
	adminID = 9
	// memberID is a member's plex.tv id.
	memberID = 7
	// pendingID is a pending account's plex.tv id.
	pendingID = 5
)

// usersDB stores an owner, an admin, a member, and a pending account.
//
// Parameters:
//   - t: The test the database belongs to.
//
// Returns:
//   - db: The database.
func usersDB(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New(filepath.Join(t.TempDir(), "users.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.ClaimOwner(t.Context(), ownerID, "owner")
	require.NoError(t, err)

	for id, name := range map[int]string{adminID: "admin", memberID: "member", pendingID: "pending"} {
		require.NoError(t, db.AddPendingUser(t.Context(), id, name))

		if id != pendingID {
			require.NoError(t, db.ApproveUser(t.Context(), id))
		}
	}

	require.NoError(t, db.SetUserRole(t.Context(), adminID, database.RoleAdmin))

	return db
}

// usersApp mounts the users routes for one caller.
//
// Parameters:
//   - t: The test the app belongs to.
//   - db: The database.
//   - caller: Who every request is made by.
//
// Returns:
//   - app: The app.
func usersApp(t *testing.T, db *database.DB, caller identity.User) *fiber.App {
	t.Helper()

	handler := New(db, db)

	app := fiber.New()
	respondtest.Sessions(t, app)
	app.Use(middlewaretest.AsCaller(caller))
	app.Get(routes.PathSettingsUsers, handler.Page)
	app.Get(routes.PathPending, handler.Pending)
	app.Post(routes.PathSettingsUsers+"/limits", handler.SaveGlobalLimits)
	app.Post(routes.PathSettingsUsers+"/:id/approve", handler.Approve)
	app.Post(routes.PathSettingsUsers+"/:id/reject", handler.Reject)
	app.Post(routes.PathSettingsUsers+"/:id/remove", handler.Remove)
	app.Post(routes.PathSettingsUsers+"/:id/role", handler.SetRole)
	app.Post(routes.PathSettingsUsers+"/:id/limits", handler.SaveUserLimits)

	return app
}

// send serves one request.
//
// Parameters:
//   - t: The test sending it.
//   - app: The app.
//   - method: The request method.
//   - target: The request target.
//   - form: The form body, nil for none.
//
// Returns:
//   - answer: What the app answered.
func send(t *testing.T, app *fiber.App, method, target string, form url.Values) reply {
	t.Helper()

	req := httptest.NewRequestWithContext(
		t.Context(),
		method,
		target,
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return reply{
		body:     string(body),
		location: resp.Header.Get(fiber.HeaderLocation),
		flash:    respondtest.Flash(t, app, resp).Message,
		status:   resp.StatusCode,
	}
}

// as is a caller with a role.
//
// Parameters:
//   - id: The caller's plex.tv id.
//   - role: The caller's role.
//
// Returns:
//   - caller: The caller.
func as(id int, role identity.Role) identity.User {
	return identity.User{PlexID: id, Username: "", Role: role}
}

// role reads a user's role.
//
// Parameters:
//   - t: The test reading it.
//   - db: The database.
//   - id: The user.
//
// Returns:
//   - role: The role, empty for a user not stored.
func role(t *testing.T, db *database.DB, id int) string {
	t.Helper()

	got, _, err := db.UserRole(t.Context(), id)
	require.NoError(t, err)

	return got
}

// TestPageShowsTheOwnerEveryChange covers the users page as the owner sees
// it, with every user and every change the owner may make.
func TestPageShowsTheOwnerEveryChange(t *testing.T) {
	t.Parallel()

	got := send(
		t,
		usersApp(t, usersDB(t), as(ownerID, identity.RoleOwner)),
		http.MethodGet,
		routes.PathSettingsUsers,
		nil,
	)

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Contains(t, got.body, "data-pending-users")
	assert.Contains(t, got.body, "/settings/users/5/approve")
	assert.Contains(t, got.body, "/settings/users/7/role")
	assert.Contains(t, got.body, "Make Admin")
	assert.Contains(t, got.body, "Make Member")
	assert.Contains(t, got.body, "/settings/users/9/remove")
	assert.Contains(t, got.body, "data-global-limits")
	assert.NotContains(t, got.body, "/settings/users/42/", "nothing changes the owner")
}

// TestPageShowsAnAdminOnlyTheirChanges covers an admin, who approves and
// manages members but leaves other admins alone.
func TestPageShowsAnAdminOnlyTheirChanges(t *testing.T) {
	t.Parallel()

	got := send(
		t,
		usersApp(t, usersDB(t), as(adminID, identity.RoleAdmin)),
		http.MethodGet,
		routes.PathSettingsUsers,
		nil,
	)

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Contains(t, got.body, "/settings/users/5/approve")
	assert.Contains(t, got.body, "/settings/users/7/remove")
	assert.NotContains(t, got.body, "Make Admin")
	assert.NotContains(t, got.body, "/settings/users/9/")
}

// TestChangesFollowWhoMakesThem covers each change through its route.
//
// A permitted change is made and returns to the page. One the caller may not
// make leaves the user as they were and says so.
func TestChangesFollowWhoMakesThem(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		caller identity.User
		target string
		form   url.Values
		user   int
		want   string
		denied bool
	}{
		"admin approves":       {caller: as(adminID, identity.RoleAdmin), target: "5/approve", user: pendingID, want: database.RoleMember},
		"admin rejects":        {caller: as(adminID, identity.RoleAdmin), target: "5/reject", user: pendingID, want: ""},
		"admin removes member": {caller: as(adminID, identity.RoleAdmin), target: "7/remove", user: memberID, want: ""},
		"admin removes admin": {
			caller: as(adminID, identity.RoleAdmin), target: "9/remove", user: adminID, want: database.RoleAdmin, denied: true,
		},
		"admin makes admin": {
			caller: as(adminID, identity.RoleAdmin), target: "7/role", form: url.Values{"role": {"admin"}},
			user: memberID, want: database.RoleMember, denied: true,
		},
		"owner makes admin": {
			caller: as(ownerID, identity.RoleOwner), target: "7/role", form: url.Values{"role": {"admin"}},
			user: memberID, want: database.RoleAdmin,
		},
		"owner makes member": {
			caller: as(ownerID, identity.RoleOwner), target: "9/role", form: url.Values{"role": {"member"}},
			user: adminID, want: database.RoleMember,
		},
		"owner removes owner": {
			caller: as(ownerID, identity.RoleOwner), target: "42/remove", user: ownerID, want: database.RoleOwner, denied: true,
		},
		"member removes member": {
			caller: as(memberID, identity.RoleMember), target: "7/remove", user: memberID, want: database.RoleMember, denied: true,
		},
		"unknown user": {
			caller: as(ownerID, identity.RoleOwner), target: "99/remove", user: 99, want: "", denied: true,
		},
		"bad id": {
			caller: as(ownerID, identity.RoleOwner), target: "x/remove", user: memberID, want: database.RoleMember, denied: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			db := usersDB(t)

			got := send(
				t,
				usersApp(t, db, test.caller),
				http.MethodPost,
				routes.PathSettingsUsers+"/"+test.target,
				test.form,
			)

			assert.Equal(t, fiber.StatusSeeOther, got.status)
			assert.Equal(t, routes.PathSettingsUsers, got.location)
			assert.Equal(t, test.want, role(t, db, test.user))

			if test.denied {
				assert.Equal(t, msgCannotChange, got.flash)
			} else {
				assert.Empty(t, got.flash)
			}
		})
	}
}

// TestSaveGlobalLimitsStoresEveryLimit covers the global limits form, and a
// value it refuses without storing anything.
func TestSaveGlobalLimitsStoresEveryLimit(t *testing.T) {
	t.Parallel()

	db := usersDB(t)
	app := usersApp(t, db, as(adminID, identity.RoleAdmin))

	got := send(t, app, http.MethodPost, routes.PathSettingsUsers+"/limits", url.Values{
		"active": {"3"}, "weekly": {"0"}, "storageGB": {"2.5"}, "clipSeconds": {"120"},
	})
	assert.Empty(t, got.flash)

	limits, err := db.GlobalLimits(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, limits.Active)
	assert.Zero(t, limits.Weekly, "0 means no limit")
	assert.Equal(t, int64(2_500_000_000), limits.Storage)
	assert.Equal(t, int64(120), int64(limits.ClipLength.Seconds()))

	refused := send(
		t,
		app,
		http.MethodPost,
		routes.PathSettingsUsers+"/limits",
		url.Values{"active": {"-1"}},
	)
	assert.Equal(t, string(errActive), refused.flash)

	again, err := db.GlobalLimits(t.Context())
	require.NoError(t, err)
	assert.Equal(t, limits, again, "a refused form changes nothing")
}

// TestSaveUserLimitsStoresAMembersOwnLimits covers one member's limits form,
// where an empty field takes the global limit.
func TestSaveUserLimitsStoresAMembersOwnLimits(t *testing.T) {
	t.Parallel()

	db := usersDB(t)
	app := usersApp(t, db, as(ownerID, identity.RoleOwner))

	got := send(t, app, http.MethodPost, routes.PathSettingsUsers+"/7/limits", url.Values{
		"active": {"5"}, "weekly": {""}, "storageGB": {""}, "clipSeconds": {""},
	})
	assert.Empty(t, got.flash)

	override, err := db.UserLimits(t.Context(), memberID)
	require.NoError(t, err)
	require.NotNil(t, override.Active)
	assert.Equal(t, 5, *override.Active)
	assert.Equal(t, quota.Override{Active: override.Active}, override)

	tooLarge := strconv.Itoa(math.MaxInt32 + 1)

	for field, refusal := range map[string]refusal{
		"weekly": errWeekly, "storageGB": errStorage, "clipSeconds": errClip,
	} {
		refused := send(
			t,
			app,
			http.MethodPost,
			routes.PathSettingsUsers+"/7/limits",
			url.Values{field: {tooLarge + "x"}},
		)
		assert.Equal(t, string(refusal), refused.flash, field)
	}

	for field, refusal := range map[string]refusal{"active": errActive, "weekly": errWeekly, "clipSeconds": errClip} {
		refused := send(
			t,
			app,
			http.MethodPost,
			routes.PathSettingsUsers+"/7/limits",
			url.Values{field: {tooLarge}},
		)
		assert.Equal(t, string(refusal), refused.flash, field+" past what a limit holds")
	}

	for field, refusal := range map[string]refusal{
		"weekly": errWeekly, "storageGB": errStorage, "clipSeconds": errClip,
	} {
		refused := send(
			t,
			app,
			http.MethodPost,
			routes.PathSettingsUsers+"/7/limits",
			url.Values{field: {"x"}},
		)
		assert.Equal(t, string(refusal), refused.flash, field)
	}
}

// TestPendingPageHoldsOnlyAPendingAccount covers the waiting page, which an
// approved account skips for the dashboard.
func TestPendingPageHoldsOnlyAPendingAccount(t *testing.T) {
	t.Parallel()

	db := usersDB(t)

	waiting := send(
		t,
		usersApp(t, db, as(pendingID, identity.RolePending)),
		http.MethodGet,
		routes.PathPending,
		nil,
	)
	assert.Equal(t, fiber.StatusOK, waiting.status)
	assert.Contains(t, waiting.body, "data-pending")
	assert.Contains(t, waiting.body, "Waiting for Approval")

	approved := send(
		t,
		usersApp(t, db, as(memberID, identity.RoleMember)),
		http.MethodGet,
		routes.PathPending,
		nil,
	)
	assert.Equal(t, fiber.StatusSeeOther, approved.status)
	assert.Equal(t, routes.PathRoot, approved.location)
}

// TestRoleNameNamesEachRole covers the badge each user carries.
func TestRoleNameNamesEachRole(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Owner", roleName(identity.RoleOwner))
	assert.Equal(t, "Admin", roleName(identity.RoleAdmin))
	assert.Equal(t, "Member", roleName(identity.RoleMember))
	assert.Equal(t, "Pending", roleName(identity.RolePending))
}

// UserRole reports the stale role for a user it holds one for.
//
// Parameters:
//   - ctx: Request context.
//   - plexUserID: The user.
//
// Returns:
//   - role: The stale or stored role.
//   - found: Whether the user is stored.
//   - err: The database's failure.
func (store staleRoles) UserRole(ctx context.Context, plexUserID int) (string, bool, error) {
	if role, ok := store.roles[plexUserID]; ok {
		return role, true, nil
	}

	//nolint:wrapcheck // The test hands the database's answer on as it is.
	return store.DB.UserRole(ctx, plexUserID)
}

// TestChangesLeaveAUserWhoseRoleChangedMeanwhile covers an admin removing a
// member whom the owner made an admin after the admin's check.
//
// The removal is refused with the page's notice, and the new admin stays.
// Setting their limits is refused the same way.
func TestChangesLeaveAUserWhoseRoleChangedMeanwhile(t *testing.T) {
	t.Parallel()

	db := usersDB(t)
	require.NoError(t, db.SetUserRole(t.Context(), memberID, database.RoleAdmin))

	handler := New(staleRoles{DB: db, roles: map[int]string{memberID: database.RoleMember}}, db)

	app := fiber.New()
	respondtest.Sessions(t, app)
	app.Use(middlewaretest.AsCaller(identity.User{
		PlexID: adminID, Username: "admin", Role: identity.RoleAdmin,
	}))
	app.Post(routes.PathSettingsUsers+"/:id/remove", handler.Remove)
	app.Post(routes.PathSettingsUsers+"/:id/limits", handler.SaveUserLimits)

	target := routes.PathSettingsUsers + "/" + strconv.Itoa(memberID)

	removed := send(t, app, http.MethodPost, target+"/remove", nil)
	assert.Equal(t, msgCannotChange, removed.flash)

	limited := send(t, app, http.MethodPost, target+"/limits", url.Values{"active": {"1"}})
	assert.Equal(t, msgCannotChange, limited.flash)

	role, found, err := db.UserRole(t.Context(), memberID)
	require.NoError(t, err)
	assert.True(t, found, "the new admin was not removed")
	assert.Equal(t, database.RoleAdmin, role)
}
