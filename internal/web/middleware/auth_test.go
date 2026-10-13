// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/quota"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/web/middleware/mocks"
)

// fakeTokens answers server token lookups with a fixed result and counts
// them.
type fakeTokens struct {
	// err is what each lookup fails with, nil for none.
	err error
	// token is what each lookup returns.
	token string
	// asked counts the lookups.
	asked int
}

// errStoreUnreadable reports a user store that cannot be read.
var errStoreUnreadable = errors.New("user store is unreadable")

// errPlexDown stands in for plex.tv that cannot be asked.
var errPlexDown = errors.New("plex.tv is down")

func TestAuthGuardPassesTheE2EEnvironmentThroughWithoutAToken(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, guardApp(t, config.EnvE2E, nil), http.MethodGet, "/dashboard/sessions")

	assert.Equal(t, fiber.StatusOK, got.status)
}

func TestAuthGuardAnswersAnonymousAPIRequestsWithAnErrorEnvelope(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, guardApp(t, "test", nil), http.MethodGet, "/api/clips")

	assert.Equal(t, fiber.StatusUnauthorized, got.status)
	assert.JSONEq(t, `{"error":"unauthorized","message":"authentication required"}`, got.body)
}

func TestAuthGuardRedirectsAnonymousPageRequestsToLogin(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"/dashboard/sessions", "/clips/42", "/"} {
		got := issueRequest(t, guardApp(t, "test", nil), http.MethodGet, target)

		assert.Equal(t, fiber.StatusSeeOther, got.status, target)
		assert.Equal(t, "/login", got.location, target)
	}
}

func TestAuthGuardPassesASessionSignedInAsAKnownUser(t *testing.T) {
	t.Parallel()

	users := mocks.NewMockUserLookup(t)
	users.EXPECT().UserRole(mock.Anything, 42).Return("owner", true, nil).Twice()

	app := guardAppWith(t, users, withUser(42))

	for _, target := range []string{"/api/clips", "/dashboard/sessions"} {
		got := issueRequest(t, app, http.MethodGet, target)

		assert.Equal(t, fiber.StatusOK, got.status, target)
	}
}

func TestAuthGuardRefusesATokenWithoutAUser(t *testing.T) {
	t.Parallel()

	// A lookup with no expectations fails the test if the guard consults it.
	users := mocks.NewMockUserLookup(t)

	got := issueRequest(t, guardAppWith(t, users, withToken("session-token")), http.MethodGet, "/")

	assert.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location)
}

func TestAuthGuardRefusesEverySessionWithoutAUserStore(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, guardAppWith(t, nil, withUser(42)), http.MethodGet, "/api/clips")

	assert.Equal(t, fiber.StatusUnauthorized, got.status)
}

func TestAuthGuardSignsOutASessionWhoseUserWasRemoved(t *testing.T) {
	t.Parallel()

	users := mocks.NewMockUserLookup(t)
	users.EXPECT().UserRole(mock.Anything, 7).Return("", false, nil).Once()

	app := chainApp(t, func(ctx fiber.Ctx) error {
		return ctx.SendString(identity.Token(session.FromContext(ctx)))
	},
		session.New(),
		withUser(7),
		func(ctx fiber.Ctx) error {
			err := AuthGuard("test", users, nil)(ctx)

			// The guard answered with a redirect. Expose what it left on the
			// session, so the test can tell the session was reset.
			ctx.Set("X-Remaining-Token", identity.Token(session.FromContext(ctx)))

			return err
		},
	)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, fiber.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get(fiber.HeaderLocation))
	assert.Empty(t, resp.Header.Get("X-Remaining-Token"), "the revoked session was reset")
}

func TestAuthGuardFailsClosedWhenTheUserCannotBeResolved(t *testing.T) {
	t.Parallel()

	users := mocks.NewMockUserLookup(t)
	users.EXPECT().UserRole(mock.Anything, 9).Return("", false, errStoreUnreadable).Once()

	got := issueRequest(t, guardAppWith(t, users, withUser(9)), http.MethodGet, "/")

	assert.Equal(t, fiber.StatusInternalServerError, got.status)
}

// TestAuthGuardRecordsTheCaller covers the caller AuthGuard hands the
// handlers behind it.
//
// A signed-in session's caller carries its stored role, and the e2e
// environment acts as the owner.
func TestAuthGuardRecordsTheCaller(t *testing.T) {
	t.Parallel()

	users := mocks.NewMockUserLookup(t)
	users.EXPECT().UserRole(mock.Anything, 7).Return("member", true, nil).Once()

	member := issueRequest(
		t,
		chainApp(t, callerHandler(), session.New(), withUser(7), AuthGuard("test", users, nil)),
		http.MethodGet,
		"/",
	)
	assert.Equal(t, "7 member", member.body)

	e2e := issueRequest(
		t,
		chainApp(t, callerHandler(), session.New(), AuthGuard(config.EnvE2E, nil, nil)),
		http.MethodGet,
		"/",
	)
	assert.Equal(t, "0 owner", e2e.body)

	anonymous := issueRequest(t, chainApp(t, callerHandler()), http.MethodGet, "/")
	assert.Equal(t, "none", anonymous.body, "a request AuthGuard never saw has no caller")
}

// TestClipScopeFollowsTheCaller covers whose clips each caller reaches.
//
// A member reaches their own, the owner and admins reach everyone's, and a
// request without a caller reaches nothing.
func TestClipScopeFollowsTheCaller(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		caller *identity.User
		want   clip.Scope
	}{
		{
			caller: &identity.User{PlexID: 7, Username: "", Role: identity.RoleMember},
			want:   clip.Scope{Owner: 7, Everyone: false},
		},
		{
			caller: &identity.User{PlexID: 42, Username: "", Role: identity.RoleOwner},
			want:   clip.Scope{Owner: 42, Everyone: true},
		},
		{
			caller: &identity.User{PlexID: 9, Username: "", Role: identity.RoleAdmin},
			want:   clip.Scope{Owner: 9, Everyone: true},
		},
		{caller: nil, want: clip.Scope{Owner: 0, Everyone: false}},
	} {
		var got clip.Scope

		app := fiber.New()
		app.Use(func(ctx fiber.Ctx) error {
			if test.caller != nil {
				SetCaller(ctx, *test.caller)
			}

			got = ClipScope(ctx)

			return ctx.SendStatus(fiber.StatusOK)
		})

		issueRequest(t, app, http.MethodGet, "/")

		assert.Equal(t, test.want, got)
	}
}

// TestPlexViewerFollowsTheCaller covers whose token each caller reads the
// bound server with.
//
// The owner reads with the bound token, anyone else with the token stored on
// their session, and a request without a caller with none.
func TestPlexViewerFollowsTheCaller(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		caller *identity.User
		want   identity.Viewer
	}{
		"owner": {
			caller: &identity.User{PlexID: 42, Username: "", Role: identity.RoleOwner},
			want:   identity.OwnerViewer(42),
		},
		"member": {
			caller: &identity.User{PlexID: 7, Username: "", Role: identity.RoleMember},
			want:   identity.Viewer{Token: "member-server-token", PlexID: 7, Owner: false},
		},
		"admin": {
			caller: &identity.User{PlexID: 9, Username: "", Role: identity.RoleAdmin},
			want:   identity.Viewer{Token: "member-server-token", PlexID: 9, Owner: false},
		},
		"nobody": {caller: nil, want: identity.Viewer{Token: "", PlexID: 0, Owner: false}},
	} {
		var got identity.Viewer

		app := chainApp(t, okHandler(), session.New(), func(ctx fiber.Ctx) error {
			identity.SetServerToken(session.FromContext(ctx), "member-server-token", time.Now())

			if test.caller != nil {
				SetCaller(ctx, *test.caller)
			}

			got = PlexViewer(ctx)

			return ctx.Next()
		})

		issueRequest(t, app, http.MethodGet, "/")

		assert.Equal(t, test.want, got, name)
	}
}

// TestAuthGuardRenewsAMembersServerToken covers asking plex.tv again for a
// member's own server token.
//
// A missing token is asked for, but no more than once a minute. A held token
// is checked again after ten minutes, and the session ends when plex.tv no
// longer lists the server. A failure to ask keeps the held token, and the
// owner is never asked about.
func TestAuthGuardRenewsAMembersServerToken(t *testing.T) {
	t.Parallel()

	now := time.Now()

	for name, test := range map[string]struct {
		role      string
		held      string
		checked   time.Time
		lookup    fakeTokens
		want      string
		asks      int
		signedOut bool
	}{
		"missing":          {role: "member", lookup: fakeTokens{token: "fresh"}, want: "fresh", asks: 1},
		"missing, retried": {role: "member", checked: now.Add(-30 * time.Second), lookup: fakeTokens{token: "fresh"}, want: "", asks: 0},
		"held, recent":     {role: "member", held: "old", checked: now.Add(-time.Minute * 5), lookup: fakeTokens{token: "fresh"}, want: "old", asks: 0},
		"held, stale":      {role: "member", held: "old", checked: now.Add(-time.Minute * 11), lookup: fakeTokens{token: "fresh"}, want: "fresh", asks: 1},
		"share removed":    {role: "member", held: "old", checked: now.Add(-time.Minute * 11), lookup: fakeTokens{token: ""}, signedOut: true, asks: 1},
		"plex.tv down":     {role: "member", held: "old", checked: now.Add(-time.Minute * 11), lookup: fakeTokens{err: errPlexDown}, want: "old", asks: 1},
		"owner":            {role: "owner", lookup: fakeTokens{token: "fresh"}, want: "", asks: 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			users := mocks.NewMockUserLookup(t)
			users.EXPECT().UserRole(mock.Anything, 7).Return(test.role, true, nil).Once()

			lookup := test.lookup

			app := chainApp(t,
				func(ctx fiber.Ctx) error {
					return ctx.SendString(identity.ServerToken(session.FromContext(ctx)))
				},
				session.New(),
				withUser(7),
				func(ctx fiber.Ctx) error {
					if !test.checked.IsZero() || test.held != "" {
						identity.SetServerToken(session.FromContext(ctx), test.held, test.checked)
					}

					return ctx.Next()
				},
				AuthGuard("test", users, &lookup),
			)

			got := issueRequest(t, app, http.MethodGet, "/")

			assert.Equal(t, test.asks, lookup.asked)

			if test.signedOut {
				assert.Equal(t, "/login", got.location, "a removed share ends the session")

				return
			}

			assert.Equal(t, test.want, got.body)
		})
	}
}

// TestAuthGuardHoldsAPendingAccountAtThePendingPage covers an account an
// admin has not approved.
//
// A page sends it to the pending page, the API refuses it, and the pending
// page itself opens.
func TestAuthGuardHoldsAPendingAccountAtThePendingPage(t *testing.T) {
	t.Parallel()

	users := mocks.NewMockUserLookup(t)
	users.EXPECT().UserRole(mock.Anything, 7).Return("pending", true, nil).Times(3)

	app := guardAppWith(t, users, withUser(7))

	page := issueRequest(t, app, http.MethodGet, "/clips")
	assert.Equal(t, fiber.StatusSeeOther, page.status)
	assert.Equal(t, "/pending", page.location)

	call := issueRequest(t, app, http.MethodGet, "/api/clips")
	assert.Equal(t, fiber.StatusForbidden, call.status)
	assert.Contains(t, call.body, `"error":"pending"`)

	assert.Equal(t, fiber.StatusOK, issueRequest(t, app, http.MethodGet, "/pending").status)
}

// TestRenderMemberExemptsOnlyManagers covers whom the render limits bind.
func TestRenderMemberExemptsOnlyManagers(t *testing.T) {
	t.Parallel()

	for role, exempt := range map[identity.Role]bool{
		identity.RoleOwner:   true,
		identity.RoleAdmin:   true,
		identity.RoleMember:  false,
		identity.RolePending: false,
	} {
		var got quota.Member

		app := fiber.New()
		app.Use(func(ctx fiber.Ctx) error {
			SetCaller(ctx, identity.User{PlexID: 7, Username: "", Role: role})

			got = RenderMember(ctx)

			return ctx.SendStatus(fiber.StatusOK)
		})

		issueRequest(t, app, http.MethodGet, "/")

		assert.Equal(t, quota.Member{ID: 7, Exempt: exempt}, got, string(role))
	}
}

// TestAdminGuardLetsOnlyTheOwnerAndAdminsThrough covers the admin pages'
// guard.
func TestAdminGuardLetsOnlyTheOwnerAndAdminsThrough(t *testing.T) {
	t.Parallel()

	for role, want := range map[string]int{
		"owner":  fiber.StatusOK,
		"admin":  fiber.StatusOK,
		"member": fiber.StatusForbidden,
	} {
		users := mocks.NewMockUserLookup(t)
		users.EXPECT().UserRole(mock.Anything, 7).Return(role, true, nil).Once()

		app := chainApp(
			t,
			okHandler(),
			session.New(),
			withUser(7),
			AuthGuard("test", users, nil),
			AdminGuard(),
		)

		assert.Equal(t, want, issueRequest(t, app, http.MethodGet, "/").status, role)
	}

	unguarded := chainApp(t, okHandler(), AdminGuard())

	assert.Equal(t, fiber.StatusForbidden, issueRequest(t, unguarded, http.MethodGet, "/").status,
		"a request without a caller is refused")
}

// TestOwnerGuardLetsOnlyTheOwnerThrough covers the Servers pages, which not
// even an admin may use.
func TestOwnerGuardLetsOnlyTheOwnerThrough(t *testing.T) {
	t.Parallel()

	for role, want := range map[string]int{
		"owner":  fiber.StatusOK,
		"admin":  fiber.StatusForbidden,
		"member": fiber.StatusForbidden,
	} {
		users := mocks.NewMockUserLookup(t)
		users.EXPECT().UserRole(mock.Anything, 7).Return(role, true, nil).Once()

		app := chainApp(
			t,
			okHandler(),
			session.New(),
			withUser(7),
			AuthGuard("test", users, nil),
			OwnerGuard(),
		)

		assert.Equal(t, want, issueRequest(t, app, http.MethodGet, "/").status, role)
	}

	unguarded := chainApp(t, okHandler(), OwnerGuard())

	assert.Equal(t, fiber.StatusForbidden, issueRequest(t, unguarded, http.MethodGet, "/").status,
		"a request without a caller is refused")
}

func TestUnauthenticatedRedirectsAPathWithoutTheAPIPrefix(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, refusingApp(t), http.MethodGet, "/clips/42")

	assert.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location)
}

func TestUnauthenticatedTreatsTheBareAPIPathAsAPage(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, refusingApp(t), http.MethodGet, "/api")

	assert.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location)
}

func TestUnauthenticatedDoesNotContinueTheChain(t *testing.T) {
	t.Parallel()

	got := issueRequest(t, refusingApp(t), http.MethodGet, "/api/clips")

	assert.Equal(t, fiber.StatusUnauthorized, got.status)
	assert.NotContains(t, got.body, "no-token")
}

// guardApp builds a session-backed application with the auth guard in front of
// the terminal handler, resolving users against an empty store.
//
// Parameters:
//   - t: The test that owns the application.
//   - env: Environment the guard is configured with.
//   - seed: Middleware run before the guard, or nil for an anonymous session.
//
// Returns:
//   - app: An application whose only route is guarded.
func guardApp(t *testing.T, env string, seed fiber.Handler) *fiber.App {
	t.Helper()

	middle := []fiber.Handler{session.New()}
	if seed != nil {
		middle = append(middle, seed)
	}

	return chainApp(
		t,
		okHandler(),
		append(middle, AuthGuard(env, mocks.NewMockUserLookup(t), nil))...,
	)
}

// guardAppWith builds a session-backed application whose auth guard resolves
// users against a given store.
//
// Parameters:
//   - t: The test that owns the application.
//   - users: User store the guard consults, which may be nil.
//   - seed: Middleware run before the guard.
//
// Returns:
//   - app: An application whose only route is guarded.
func guardAppWith(t *testing.T, users UserLookup, seed fiber.Handler) *fiber.App {
	t.Helper()

	return chainApp(t, okHandler(), session.New(), seed, AuthGuard("test", users, nil))
}

// refusingApp builds an application that answers every request with
// unauthenticated and never reaches the handler behind it.
//
// Parameters:
//   - t: The test that owns the application.
//
// Returns:
//   - app: An application whose only handler rejects the request.
func refusingApp(t *testing.T) *fiber.App {
	t.Helper()

	return chainApp(t, writeHandler("no-token"), func(ctx fiber.Ctx) error {
		return unauthenticated(ctx)
	})
}

// withToken seeds the session with a Plex access token before the guard runs.
//
// Parameters:
//   - token: Plex access token to store on the session.
//
// Returns:
//   - handler: Middleware that seeds the session and continues the chain.
func withToken(token string) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		identity.SetToken(session.FromContext(ctx), token)

		return ctx.Next()
	}
}

// withUser seeds the session with a Plex access token and the Plex user id it
// signed in as, before the guard runs.
//
// Parameters:
//   - plexUserID: Plex user id to store on the session.
//
// Returns:
//   - handler: Middleware that seeds the session and continues the chain.
func withUser(plexUserID int) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		sess := session.FromContext(ctx)
		identity.SetToken(sess, "session-token")
		identity.SetUserID(sess, plexUserID)

		return ctx.Next()
	}
}

// callerHandler answers with the caller AuthGuard recorded.
//
// Returns:
//   - handler: A terminal handler writing the caller's Plex id and role, or
//     "none".
func callerHandler() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		caller, ok := Caller(ctx)
		if !ok {
			return ctx.SendString("none")
		}

		return ctx.SendString(strconv.Itoa(caller.PlexID) + " " + string(caller.Role))
	}
}

// ServerToken returns the fake's result and counts the lookup.
//
// Returns:
//   - token: The fake's token.
//   - err: The fake's err.
func (tokens *fakeTokens) ServerToken(context.Context, string) (string, error) {
	tokens.asked++

	return tokens.token, tokens.err
}
