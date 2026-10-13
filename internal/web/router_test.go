// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	clippreview "github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// routerRoute is one row of the route table New is expected to register.
type routerRoute struct {
	// method is the HTTP method the route answers.
	method string
	// path is the registered route pattern.
	path string
	// guarded is true when the auth guard sits in front of the handler.
	guarded bool
	// admin is true when the admin guard follows the auth guard.
	admin bool
	// sample is a concrete request path with the parameters filled in.
	sample string
}

// answer is everything a router test observes about one response.
type answer struct {
	// status is the response status code.
	status int
	// location is the Location response header.
	location string
	// contentType is the Content-Type response header.
	contentType string
	// body is the response body, read to end.
	body string
	// cookies are the cookies the application set.
	cookies []*http.Cookie
}

// browser drives the router the way a browser would, carrying the cookies and
// the CSRF token the application handed out.
type browser struct {
	// app is the router under test.
	app *fiber.App
	// db is the store the owner and the selected server live in.
	db *database.DB
	// cookies are the cookies the application set.
	cookies []*http.Cookie
	// csrfToken is the token the CSRF cookie carries.
	csrfToken string
	// host is the Host header the browser sends.
	host string
	// bind is the router's server binding, unbound until a test binds it.
	bind *identity.Binding
	// plexURL is where the plex.tv stand-in, which also stands in for the
	// bound server, answers.
	plexURL string
}

const (
	// ownerToken is the Plex token the plex.tv stand-in maps to account 42.
	ownerToken = "plex-token"

	// otherToken is the Plex token the plex.tv stand-in maps to account 7.
	otherToken = "other-token"

	// sharedTitle is the item the bound server stand-in shares with account 7.
	sharedTitle = "501"

	// memberServerToken is account 7's token for the bound server.
	memberServerToken = "member-server-token"

	// routerHost is the Host header every browser request carries.
	routerHost = "localhost"

	// browserTimeout bounds one browser request. The default second is too
	// short on a loaded CI runner under the race detector.
	browserTimeout = 30 * time.Second
)

// clipReadRoutes are the clip routes that read a clip without changing it.
//
// Every other route naming a clip by id changes it.
var clipReadRoutes = map[string]bool{
	http.MethodGet + " /clips/:id/file":         true,
	http.MethodGet + " /clips/:id/sdr":          true,
	http.MethodGet + " /clips/:id/row":          true,
	http.MethodGet + " /api/clips/:id/status":   true,
	http.MethodGet + " /api/clips/:id/download": true,
}

// wantRoutes is the complete route table New must register, in registration
// order, with each route classified as guarded or open.
var wantRoutes = []routerRoute{
	{method: http.MethodGet, path: "/login", sample: "/login"},
	{method: http.MethodGet, path: "/", guarded: true, sample: "/"},
	{
		method:  http.MethodGet,
		path:    "/dashboard/sessions",
		guarded: true,
		sample:  "/dashboard/sessions",
	},
	{method: http.MethodGet, path: "/media", guarded: true, sample: "/media"},
	{
		method:  http.MethodGet,
		path:    "/media/item/:id/playback",
		guarded: true,
		sample:  "/media/item/5/playback",
	},
	{
		method:  http.MethodGet,
		path:    "/media/item/:id/position",
		guarded: true,
		sample:  "/media/item/5/position",
	},
	{
		method:  http.MethodGet,
		path:    "/media/item/:id/clips",
		guarded: true,
		sample:  "/media/item/5/clips",
	},
	{method: http.MethodGet, path: "/media/item/:id", guarded: true, sample: "/media/item/5"},
	{method: http.MethodGet, path: "/previews/:id", guarded: true, sample: "/previews/9"},
	{method: http.MethodGet, path: "/nav/libraries", guarded: true, sample: "/nav/libraries"},
	{method: http.MethodGet, path: "/thumbs", guarded: true, sample: "/thumbs"},
	{method: http.MethodGet, path: "/clips/new", guarded: true, sample: "/clips/new"},
	{method: http.MethodGet, path: "/clips/:id/file", guarded: true, sample: "/clips/7/file"},
	{method: http.MethodGet, path: "/clips/:id/sdr", guarded: true, sample: "/clips/7/sdr"},
	{method: http.MethodGet, path: "/clips/:id/row", guarded: true, sample: "/clips/7/row"},
	{method: http.MethodGet, path: routeClips, guarded: true, sample: routeClips},
	{method: http.MethodGet, path: "/live", guarded: true, sample: "/live"},
	{method: http.MethodGet, path: "/servers", guarded: true, admin: true, sample: "/servers"},
	{
		method:  http.MethodGet,
		path:    "/settings/appearance",
		guarded: true,
		sample:  "/settings/appearance",
	},
	{
		method:  http.MethodGet,
		path:    "/settings/previews",
		guarded: true,
		admin:   true,
		sample:  "/settings/previews",
	},
	{
		method:  http.MethodGet,
		path:    "/settings/profiles",
		guarded: true,
		admin:   true,
		sample:  "/settings/profiles",
	},
	{method: http.MethodPost, path: "/servers", guarded: true, admin: true, sample: "/servers"},
	{
		method:  http.MethodPost,
		path:    "/servers/forget",
		guarded: true,
		admin:   true,
		sample:  "/servers/forget",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/previews",
		guarded: true,
		admin:   true,
		sample:  "/settings/previews",
	},
	{
		method:  http.MethodGet,
		path:    "/settings/storage",
		guarded: true,
		admin:   true,
		sample:  "/settings/storage",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/storage/previews/clear",
		guarded: true,
		admin:   true,
		sample:  "/settings/storage/previews/clear",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/storage/thumbnails/clear",
		guarded: true,
		admin:   true,
		sample:  "/settings/storage/thumbnails/clear",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/profiles",
		guarded: true,
		admin:   true,
		sample:  "/settings/profiles",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/profiles/:id/default",
		guarded: true,
		admin:   true,
		sample:  "/settings/profiles/abc/default",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/profiles/:id/delete",
		guarded: true,
		admin:   true,
		sample:  "/settings/profiles/abc/delete",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/profiles/:id",
		guarded: true,
		admin:   true,
		sample:  "/settings/profiles/abc",
	},
	{method: http.MethodPost, path: "/api/clips", guarded: true, sample: "/api/clips"},
	{
		method:  http.MethodPost,
		path:    "/api/clips/preview",
		guarded: true,
		sample:  "/api/clips/preview",
	},
	{
		method:  http.MethodPost,
		path:    "/api/clips/bulk",
		guarded: true,
		sample:  "/api/clips/bulk",
	},
	{
		method:  http.MethodPost,
		path:    "/api/clips/retry",
		guarded: true,
		sample:  "/api/clips/retry",
	},
	{
		method:  http.MethodGet,
		path:    "/api/clips/preview/:id",
		guarded: true,
		sample:  "/api/clips/preview/abc",
	},
	{
		method:  http.MethodDelete,
		path:    "/api/clips/preview/:id",
		guarded: true,
		sample:  "/api/clips/preview/abc",
	},
	{
		method:  http.MethodPost,
		path:    "/api/clips/:id/update",
		guarded: true,
		sample:  "/api/clips/abc/update",
	},
	{
		method:  http.MethodPost,
		path:    "/api/clips/:id/cancel",
		guarded: true,
		sample:  "/api/clips/abc/cancel",
	},
	{
		method:  http.MethodPost,
		path:    "/api/clips/:id/share",
		guarded: true,
		sample:  "/api/clips/abc/share",
	},
	{
		method:  http.MethodPost,
		path:    "/api/clips/:id/unshare",
		guarded: true,
		sample:  "/api/clips/abc/unshare",
	},
	{method: http.MethodGet, path: "/api/clips", guarded: true, sample: "/api/clips"},
	{
		method:  http.MethodGet,
		path:    "/api/clips/:id/status",
		guarded: true,
		sample:  "/api/clips/abc/status",
	},
	{
		method:  http.MethodGet,
		path:    "/api/clips/:id/download",
		guarded: true,
		sample:  "/api/clips/abc/download",
	},
	{method: http.MethodDelete, path: "/api/clips/:id", guarded: true, sample: "/api/clips/abc"},
	{method: http.MethodGet, path: "/api/media/search", guarded: true, sample: "/api/media/search"},
	{method: http.MethodGet, path: "/api/sessions", guarded: true, sample: "/api/sessions"},
	{method: http.MethodPost, path: "/api/auth/login", sample: "/api/auth/login"},
	{method: http.MethodGet, path: "/api/auth/callback", sample: "/api/auth/callback"},
	{method: http.MethodGet, path: "/api/auth/status", sample: "/api/auth/status"},
	{method: http.MethodPost, path: "/api/auth/logout", sample: "/api/auth/logout"},
	{method: http.MethodGet, path: "/api/healthz", sample: "/api/healthz"},
	{method: http.MethodGet, path: "/pending", guarded: true, sample: "/pending"},
	{
		method:  http.MethodGet,
		path:    "/settings/users",
		guarded: true,
		admin:   true,
		sample:  "/settings/users",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/users/limits",
		guarded: true,
		admin:   true,
		sample:  "/settings/users/limits",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/users/:id/approve",
		guarded: true,
		admin:   true,
		sample:  "/settings/users/7/approve",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/users/:id/reject",
		guarded: true,
		admin:   true,
		sample:  "/settings/users/7/reject",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/users/:id/remove",
		guarded: true,
		admin:   true,
		sample:  "/settings/users/7/remove",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/users/:id/role",
		guarded: true,
		admin:   true,
		sample:  "/settings/users/7/role",
	},
	{
		method:  http.MethodPost,
		path:    "/settings/users/:id/limits",
		guarded: true,
		admin:   true,
		sample:  "/settings/users/7/limits",
	},
}

func TestNewRegistersExactlyTheExpectedRouteTable(t *testing.T) {
	t.Parallel()

	registered := make([]string, 0, len(wantRoutes))

	for _, route := range newBrowser(t).app.GetRoutes(true) {
		registered = append(registered, route.Method+" "+route.Path)
	}

	want := make([]string, 0, len(wantRoutes))

	for _, route := range wantRoutes {
		want = append(want, route.method+" "+route.path)
	}

	assert.Len(t, registered, len(wantRoutes), "one registered route per expected row")
	assert.ElementsMatch(t, want, registered)
}

func TestNewKeepsTheClipRouteConstantAndItsLiteralInSync(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/clips", routeClips)

	registered := make([]string, 0, len(wantRoutes))

	for _, route := range newBrowser(t).app.GetRoutes(true) {
		registered = append(registered, route.Method+" "+route.Path)
	}

	assert.Contains(t, registered, http.MethodGet+" "+"/clips")
	assert.Contains(t, registered, http.MethodGet+" "+"/api/clips")
	assert.Contains(t, registered, http.MethodPost+" "+"/api/clips")
}

func TestNewMountsTheAuthGuardOnEveryGuardedRoute(t *testing.T) {
	t.Parallel()

	handlers := make(map[string]int)

	for _, route := range newBrowser(t).app.GetRoutes(true) {
		handlers[route.Method+" "+route.Path] = len(route.Handlers)
	}

	for _, route := range wantRoutes {
		want := 1
		if route.guarded {
			want = 2
		}

		if route.admin {
			want = 3
		}

		assert.Equal(t, want, handlers[route.method+" "+route.path],
			"%s %s", route.method, route.path)
	}
}

func TestNewBlocksAnonymousRequestsToEveryGuardedRoute(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, route := range wantRoutes {
		if !route.guarded {
			continue
		}

		got := client.send(t, route)

		if strings.HasPrefix(route.path, "/api/") {
			assert.Equal(t, fiber.StatusUnauthorized, got.status,
				"%s %s", route.method, route.path)

			continue
		}

		assert.Equal(t, fiber.StatusSeeOther, got.status,
			"%s %s", route.method, route.path)
		assert.Equal(t, "/login", got.location, route.path)
	}
}

func TestNewAdmitsAuthenticatedRequestsToEveryGuardedRoute(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.authenticate(t)

	for _, route := range wantRoutes {
		if !route.guarded {
			continue
		}

		got := client.send(t, route)

		assert.NotEqual(t, fiber.StatusUnauthorized, got.status,
			"%s %s", route.method, route.path)
		assert.NotEqual(t, "/login", got.location,
			"%s %s", route.method, route.path)
	}
}

func TestNewKeepsASecondBrowserAnonymous(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)

	stranger := owner.sibling(t)

	got := stranger.request(t, http.MethodGet, "/")

	assert.Equal(t, fiber.StatusSeeOther, got.status,
		"signing in one browser does not sign in every other")
	assert.Equal(t, "/login", got.location)
}

func TestNewRefusesASecondPlexAccount(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)

	intruder := owner.sibling(t)

	got := intruder.submit(t, "/api/auth/login", url.Values{"token": {otherToken}})

	require.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location,
		"the second account is sent back to the login page, which the session tells why")
	assert.Equal(t, "/login", intruder.request(t, http.MethodGet, "/").location,
		"the refused account holds no session")
}

// TestNewRefusesAMemberEveryAdminRoute covers a member, who may use Outtake
// but not manage it.
//
// Every admin route answers 403, and a page every user may open still works.
func TestNewRefusesAMemberEveryAdminRoute(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)

	member := owner.member(t)

	for _, route := range wantRoutes {
		if !route.admin {
			continue
		}

		assert.Equal(t, fiber.StatusForbidden, member.send(t, route).status,
			"%s %s", route.method, route.path)
	}

	assert.Equal(t, fiber.StatusOK, member.request(t, http.MethodGet, "/").status,
		"a member still opens the dashboard")
}

// TestNewKeepsAMemberOutOfTheOwnersClipsAndPreviews covers every route that
// names a clip or a preview by id.
//
// A member asking for the owner's clip or preview gets the answer an id
// nothing uses gets, so the record cannot be read, changed, or shown to
// exist. Lists and the page stream leave it out too, while the owner still
// sees it. The clip feed is covered by the clip handler's tests, because its
// stream stays open.
func TestNewKeepsAMemberOutOfTheOwnersClipsAndPreviews(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)

	stamp := time.Now().UTC()
	ownersClip := &clip.Job{
		ID:         "owners-clip",
		Type:       clip.TypeClip,
		Name:       "The Owner's Clip",
		MediaID:    "5",
		MediaTitle: "Owner Movie",
		MediaType:  clip.DefaultMediaType,
		CreatedAt:  stamp,
		UpdatedAt:  stamp,
		OwnerID:    42,
		Status:     clip.StatusCompleted,
	}
	require.NoError(t, owner.db.SaveClip(t.Context(), ownersClip))

	ownersPreview := clippreview.OwnedID(42, strings.Repeat("a", 64))
	member := owner.member(t)

	for _, route := range wantRoutes {
		target, absent, ok := ownedTargets(route, ownersClip.ID, ownersPreview)
		if !ok {
			continue
		}

		got := member.request(t, route.method, target)
		want := member.request(t, route.method, absent)

		assert.Equal(t, want.status, got.status, "%s %s", route.method, route.path)
		assert.Equal(t, want.location, got.location, "%s %s", route.method, route.path)
		assert.NotContains(t, got.body, ownersClip.Name, "%s %s", route.method, route.path)
	}

	assert.NotContains(t, member.request(t, http.MethodGet, "/clips").body, ownersClip.Name)
	assert.NotContains(t, member.request(t, http.MethodGet, "/api/clips").body, ownersClip.ID)
	assert.Equal(t, fiber.StatusNoContent,
		member.request(t, http.MethodGet, "/live?preview="+ownersPreview).status,
		"the stream follows none of another account's previews")

	assert.Contains(t, owner.request(t, http.MethodGet, "/api/clips").body, ownersClip.ID,
		"the owner still sees the clip")
}

// TestNewShowsAMemberOnlyTheSharedClipsTheyMaySee covers every route that
// names a clip by id, for a member and three of the owner's clips.
//
// A private clip, and a shared clip of a title Plex does not share with the
// member, answer every route as an id nothing uses does. A shared clip of a
// title the member sees answers every read route as the member's own clip
// does, and every change route as an id nothing uses does. Its card offers
// no change, and the Shared filter lists it.
func TestNewShowsAMemberOnlyTheSharedClipsTheyMaySee(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)

	member := owner.member(t)
	output := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(output, []byte("clip"), 0o600))

	stamp := time.Now().UTC()
	saved := func(id string, ownerID int, mediaID string, shared bool) *clip.Job {
		job := &clip.Job{
			ID:         id,
			Type:       clip.TypeClip,
			Name:       "Clip " + id,
			MediaID:    mediaID,
			MediaTitle: "Movie",
			MediaType:  clip.DefaultMediaType,
			OutputPath: output,
			CreatedAt:  stamp,
			UpdatedAt:  stamp,
			OwnerID:    ownerID,
			Status:     clip.StatusCompleted,
			Shared:     shared,
		}
		require.NoError(t, owner.db.SaveClip(t.Context(), job))

		return job
	}

	mine := saved("members-clip", 7, sharedTitle, false)
	seen := saved("shared-seen", 42, sharedTitle, true)
	hidden := saved("shared-hidden", 42, "502", true)
	private := saved("owners-private", 42, sharedTitle, false)

	for _, route := range wantRoutes {
		if !strings.Contains(route.path, ":id") || strings.Contains(route.path, "preview") ||
			!strings.Contains(route.path, "clips/") {

			continue
		}

		name := route.method + " " + route.path
		fill := func(id string) string { return strings.Replace(route.path, ":id", id, 1) }
		absent := member.request(t, route.method, fill("no-such-clip"))

		for _, refused := range []*clip.Job{hidden, private} {
			got := member.request(t, route.method, fill(refused.ID))

			assert.Equal(t, absent.status, got.status, "%s on %s", name, refused.ID)
			assert.NotContains(t, got.body, refused.Name, "%s on %s", name, refused.ID)
		}

		got := member.request(t, route.method, fill(seen.ID))

		if clipReadRoutes[name] {
			own := member.request(t, route.method, fill(mine.ID))

			assert.Equal(t, own.status, got.status, "%s reads a shared clip", name)

			continue
		}

		assert.Equal(t, absent.status, got.status, "%s changes no shared clip", name)
	}

	row := member.request(t, http.MethodGet, "/clips/"+seen.ID+"/row")
	require.Equal(t, fiber.StatusOK, row.status)
	assert.Contains(t, row.body, "By owner")
	assert.NotContains(t, row.body, "Regenerate")
	assert.NotContains(t, row.body, "Delete")

	listed := member.request(t, http.MethodGet, "/clips?show=shared").body
	assert.Contains(t, listed, seen.Name)
	assert.NotContains(t, listed, hidden.Name)
	assert.NotContains(t, listed, private.Name)
	assert.NotContains(t, member.request(t, http.MethodGet, "/clips").body, seen.Name,
		"the member's own list holds only their clips")
	assert.NotContains(t, member.request(t, http.MethodGet, "/api/clips").body, seen.ID)

	still, err := owner.db.GetClip(t.Context(), seen.ID)
	require.NoError(t, err)
	assert.True(t, still.Shared, "the member could not make the clip private")
}

// TestNewLetsTheOwnerShareAClip covers the share routes for a clip's owner.
//
// Sharing makes the clip visible to a member who sees its title, and making
// it private again hides it.
func TestNewLetsTheOwnerShareAClip(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)

	member := owner.member(t)
	stamp := time.Now().UTC()
	job := &clip.Job{
		ID:         "owners-clip",
		Type:       clip.TypeClip,
		Name:       "The Owner's Clip",
		MediaID:    sharedTitle,
		MediaTitle: "Movie",
		MediaType:  clip.DefaultMediaType,
		CreatedAt:  stamp,
		UpdatedAt:  stamp,
		OwnerID:    42,
		Status:     clip.StatusCompleted,
	}
	require.NoError(t, owner.db.SaveClip(t.Context(), job))

	status := "/api/clips/" + job.ID + "/status"
	absent := member.request(t, http.MethodGet, "/api/clips/no-such-clip/status").status

	require.Equal(t, absent, member.request(t, http.MethodGet, status).status)

	shared := owner.request(t, http.MethodPost, "/api/clips/"+job.ID+"/share")
	require.Equal(t, fiber.StatusSeeOther, shared.status, "a form post is sent back")
	assert.Equal(t, fiber.StatusOK, member.request(t, http.MethodGet, status).status)
	assert.Contains(t, member.request(t, http.MethodGet, status).body, `"shared":true`)

	unshared := owner.request(t, http.MethodPost, "/api/clips/"+job.ID+"/unshare")
	require.Equal(t, fiber.StatusSeeOther, unshared.status)
	assert.Equal(t, absent, member.request(t, http.MethodGet, status).status)
}

// TestNewHoldsAPendingAccountAtThePendingPage covers an account the bound
// server is shared with, signing in before an admin approves it.
//
// It is sent to the pending page, and every other guarded route sends it
// back there or, for the API, refuses it.
func TestNewHoldsAPendingAccountAtThePendingPage(t *testing.T) {
	t.Parallel()

	owner := newBrowser(t)
	owner.authenticate(t)
	owner.bindSharedServer(t)

	pending := owner.sibling(t)

	got := pending.submit(t, "/api/auth/login", url.Values{"token": {otherToken}})
	require.Equal(t, fiber.StatusSeeOther, got.status)
	require.Equal(t, "/pending", got.location)

	role, _, err := owner.db.UserRole(t.Context(), 7)
	require.NoError(t, err)
	assert.Equal(t, database.RolePending, role, "the account is stored as pending")

	assert.Equal(t, fiber.StatusOK, pending.request(t, http.MethodGet, "/pending").status)

	for _, route := range wantRoutes {
		if !route.guarded || route.path == "/pending" {
			continue
		}

		got := pending.send(t, route)

		if strings.HasPrefix(route.path, "/api/") {
			assert.Equal(t, fiber.StatusForbidden, got.status, "%s %s", route.method, route.path)

			continue
		}

		assert.Equal(t, "/pending", got.location, "%s %s", route.method, route.path)
	}
}

// TestWireLimitsReachesEveryRenderingHandler covers the enforcer New hands
// out, so a member's clips and previews are never left without limits.
func TestWireLimitsReachesEveryRenderingHandler(t *testing.T) {
	t.Parallel()

	deps := testRouterDeps(t, testRouterDatabase(t))
	built := newRouterHandlers(deps)
	limits := newLimits(deps)

	require.NotNil(t, limits)

	wireLimits(built, limits)

	assert.Same(t, limits, built.clip.Limits())
	assert.Same(t, limits, built.preview.Limits())
}

func TestNewLogoutEndsOnlyThatBrowsersSession(t *testing.T) {
	t.Parallel()

	first := newBrowser(t)
	first.authenticate(t)

	second := first.sibling(t)
	second.authenticate(t)

	got := first.submit(t, "/api/auth/logout", nil)

	require.Equal(t, fiber.StatusSeeOther, got.status)
	assert.Equal(t, "/login", got.location)
	assert.Equal(t, "/login", first.request(t, http.MethodGet, "/").location,
		"the browser that logged out is anonymous again")
	assert.Equal(t, fiber.StatusOK, second.request(t, http.MethodGet, "/").status,
		"the other browser stays signed in")

	owned, err := first.db.HasOwner(t.Context())
	require.NoError(t, err)
	assert.True(t, owned, "logging out keeps the owner")
}

func TestNewLogsOutAnExpiredSessionToo(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	got := client.submit(t, "/api/auth/logout", nil)

	assert.Equal(t, fiber.StatusSeeOther, got.status,
		"logging out never needs a live session, only a CSRF token")
	assert.Equal(t, "/login", got.location)
}

func TestNewKeepsTheSessionAcrossARestart(t *testing.T) {
	t.Parallel()

	before := newBrowser(t)
	before.authenticate(t)

	// A second router over the same database stands in for the restarted
	// process. Only the cookies carry over.
	after := &browser{
		app:       New(testRouterDeps(t, before.db)),
		db:        before.db,
		cookies:   before.cookies,
		csrfToken: before.csrfToken,
		host:      routerHost,
	}

	assert.Equal(t, fiber.StatusOK, after.request(t, http.MethodGet, "/").status,
		"the signed-in session survives the restart")

	got := after.submit(t, "/api/auth/logout", nil)
	assert.Equal(t, fiber.StatusSeeOther, got.status,
		"the CSRF token survives the restart")
	assert.Equal(t, "/login", got.location)
}

func TestNewServesAssetsAndHealthWithoutASession(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)

	for target, status := range map[string]int{
		"/assets/css/output.css":  fiber.StatusOK,
		"/assets/css/missing.css": fiber.StatusNotFound,
		"/api/healthz":            fiber.StatusOK,
	} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, status, got.status, target)
		assert.Empty(t, got.cookies, "%s sets no cookie", target)
	}

	var stored int

	err := client.db.Conn().
		QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sessions`).
		Scan(&stored)
	require.NoError(t, err)
	assert.Zero(t, stored, "no session was written")
}

func TestNewRefusesAForeignHost(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)

	client.host = "rebind.attacker.example"

	for _, target := range []string{"/login", "/api/auth/status", "/assets/css/output.css"} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, fiber.StatusMisdirectedRequest, got.status, target)
	}

	assert.Equal(t, fiber.StatusOK, client.request(t, http.MethodGet, "/api/healthz").status,
		"probes reach the health check under any host")
}

func TestNewServesTheOpenRoutesWithoutASession(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, route := range wantRoutes {
		if route.guarded {
			continue
		}

		got := client.send(t, route)

		assert.NotEqual(t, fiber.StatusUnauthorized, got.status,
			"%s %s", route.method, route.path)
	}
}

func TestNewAnswersTheHealthCheckWithoutASession(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/api/healthz")

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.JSONEq(t, `{"status":"ok","service":"outtake"}`, got.body)
}

func TestNewRendersTheSessionPagesOnceSignedIn(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.authenticate(t)

	for _, target := range []string{"/", routeClips, "/media", "/settings/profiles"} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, fiber.StatusOK, got.status, target)
	}
}

func TestNewResolvesTheMediaItemRoutesWithoutShadowing(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, target := range []string{"/media/item/5", "/media/item/5/clips", "/media/item/5/playback"} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, fiber.StatusSeeOther, got.status, target)
		assert.Equal(t, "/login", got.location, target)
	}

	got := client.request(t, http.MethodGet, "/media/item/5/unknown")

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"no media item route claims an extra path segment")
}

func TestNewResolvesTheClipRoutesWithoutShadowing(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, target := range []string{routeClips, "/clips/new", "/clips/7/file", "/clips/7/row"} {
		got := client.request(t, http.MethodGet, target)

		assert.Equal(t, fiber.StatusSeeOther, got.status, target)
		assert.Equal(t, "/login", got.location, target)
	}

	got := client.request(t, http.MethodGet, "/clips/7/unknown")

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"the clip parameter is not a catch-all")
}

func TestNewResolvesTheAPIClipRoutesWithoutShadowing(t *testing.T) {
	t.Parallel()

	client := newBrowser(t)
	client.start(t)

	for _, route := range []routerRoute{
		{method: http.MethodGet, path: "/api/clips", sample: "/api/clips"},
		{method: http.MethodPost, path: "/api/clips/preview", sample: "/api/clips/preview"},
		{method: http.MethodPost, path: "/api/clips/bulk", sample: "/api/clips/bulk"},
		{method: http.MethodPost, path: "/api/clips/retry", sample: "/api/clips/retry"},
		{method: http.MethodGet, path: "/api/clips/preview/:id", sample: "/api/clips/preview/abc"},
		{method: http.MethodDelete, path: "/api/clips/preview/:id", sample: "/api/clips/preview/abc"},
		{method: http.MethodPost, path: "/api/clips/:id/update", sample: "/api/clips/abc/update"},
		{method: http.MethodPost, path: "/api/clips/:id/cancel", sample: "/api/clips/abc/cancel"},
		{method: http.MethodPost, path: "/api/clips/:id/share", sample: "/api/clips/abc/share"},
		{method: http.MethodPost, path: "/api/clips/:id/unshare", sample: "/api/clips/abc/unshare"},
		{method: http.MethodGet, path: "/api/clips/:id/status", sample: "/api/clips/abc/status"},
		{method: http.MethodGet, path: "/api/clips/:id/download", sample: "/api/clips/abc/download"},
		{method: http.MethodDelete, path: "/api/clips/:id", sample: "/api/clips/abc"},
	} {
		got := client.send(t, route)

		assert.Equal(t, fiber.StatusUnauthorized, got.status,
			"%s %s", route.method, route.path)
	}

	got := client.request(t, http.MethodDelete, "/api/clips/abc/unknown")

	assert.Equal(t, fiber.StatusNotFound, got.status,
		"the API clip parameter is not a catch-all")
}

func TestNewRejectsAnUnregisteredPath(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/no/such/page")

	assert.Equal(t, fiber.StatusNotFound, got.status)
}

func TestNewRejectsAWrongMethodOnARegisteredPath(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/api/auth/logout")

	assert.Equal(t, fiber.StatusMethodNotAllowed, got.status)
}

func TestNewServesTheEmbeddedAssetsWithoutASession(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/assets/css/output.css")

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.Contains(t, got.contentType, "text/css")
	assert.NotEmpty(t, got.body)
}

func TestNewRouterHandlersBuildsEveryMountedHandler(t *testing.T) {
	t.Parallel()

	built := newRouterHandlers(testRouterDeps(t, testRouterDatabase(t)))

	assert.NotNil(t, built.clip)
	assert.NotNil(t, built.preview)
	assert.NotNil(t, built.media)
	assert.NotNil(t, built.auth)
	assert.NotNil(t, built.home)
	assert.NotNil(t, built.library)
	assert.NotNil(t, built.server)
	assert.NotNil(t, built.thumb)
	assert.NotNil(t, built.profiles)
	assert.NotNil(t, built.health)
}

// authenticate signs the browser in as the owner through the login form,
// against the plex.tv stand-in.
//
// Parameters:
//   - t: The test that signs in.
func (b *browser) authenticate(t *testing.T) {
	t.Helper()

	b.start(t)

	got := b.submit(t, "/api/auth/login", url.Values{"token": {ownerToken}})
	require.Equal(t, fiber.StatusSeeOther, got.status)
	require.False(t, strings.HasPrefix(got.location, "/login"),
		"the sign-in was refused: %s", got.location)
}

// bindSharedServer binds machine-1, the server the plex.tv stand-in lists as
// shared with the other account.
//
// Parameters:
//   - t: The test that binds it.
func (b *browser) bindSharedServer(t *testing.T) {
	t.Helper()

	server, ok := plex.ParseServerURL(b.plexURL)
	require.True(t, ok)

	server.Name = "Attic"
	server.Token = "owner-server-token"
	server.MachineID = "machine-1"
	b.bind.Set(server)
}

// do sends a form request the way a browser would, keeping the cookies the
// response sets.
//
// Parameters:
//   - t: The test that issues the request.
//   - method: HTTP method of the request.
//   - target: Request path.
//   - form: Form body to send.
//
// Returns:
//   - got: The response the router produced.
func (b *browser) do(t *testing.T, method, target string, form url.Values) answer {
	t.Helper()

	req := httptest.NewRequestWithContext(
		t.Context(),
		method,
		target,
		strings.NewReader(form.Encode()),
	)

	req.Host = b.host
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set("X-Csrf-Token", b.csrfToken)

	for _, cookie := range b.cookies {
		req.AddCookie(cookie)
	}

	resp, err := b.app.Test(req, browserTestConfig())
	require.NoError(t, err)

	got := readAnswer(t, resp)
	b.keep(got.cookies)

	return got
}

// keep stores the cookies a response set, replacing any of the same name the
// way a browser does, and follows a CSRF token the response rotated.
//
// Parameters:
//   - set: Cookies the response set.
func (b *browser) keep(set []*http.Cookie) {
	for _, cookie := range set {
		b.cookies = slices.DeleteFunc(b.cookies, func(held *http.Cookie) bool {
			return held.Name == cookie.Name
		})
		b.cookies = append(b.cookies, cookie)

		if cookie.Name == "csrf_" {
			b.csrfToken = cookie.Value
		}
	}
}

// member signs a second browser in as a member, the account the plex.tv
// stand-in maps otherToken to.
//
// Parameters:
//   - t: The test that signs in.
//
// Returns:
//   - member: A browser signed in as the member, sharing b's server.
func (b *browser) member(t *testing.T) *browser {
	t.Helper()

	_, err := b.db.Conn().ExecContext(
		t.Context(),
		"INSERT INTO users (plex_user_id, username, role) VALUES (7, 'friend', 'member')",
	)
	require.NoError(t, err)

	b.bindSharedServer(t)

	member := b.sibling(t)

	got := member.submit(t, "/api/auth/login", url.Values{"token": {otherToken}})
	require.Equal(t, fiber.StatusSeeOther, got.status)
	require.False(t, strings.HasPrefix(got.location, "/login"),
		"the member was refused: %s", got.location)

	return member
}

// request issues one request, carrying the session cookies and the CSRF token.
//
// Parameters:
//   - t: The test that issues the request.
//   - method: HTTP method of the request.
//   - target: Request path.
//
// Returns:
//   - got: The response the router produced.
func (b *browser) request(t *testing.T, method, target string) answer {
	t.Helper()

	form := url.Values{}
	form.Set("_csrf", b.csrfToken)

	return b.do(t, method, target, form)
}

// send issues the request a route row stands for.
//
// Parameters:
//   - t: The test that issues the request.
//   - route: The route row naming the method and the concrete path.
//
// Returns:
//   - got: The response the router produced.
func (b *browser) send(t *testing.T, route routerRoute) answer {
	t.Helper()

	return b.request(t, route.method, route.sample)
}

// sibling builds a second browser against the same router and database, with
// cookies of its own.
//
// Parameters:
//   - t: The test that owns the browser.
//
// Returns:
//   - other: A browser that shares nothing with b but the server.
func (b *browser) sibling(t *testing.T) *browser {
	t.Helper()

	other := &browser{
		app:       b.app,
		db:        b.db,
		cookies:   nil,
		csrfToken: "",
		host:      routerHost,
		bind:      b.bind,
		plexURL:   b.plexURL,
	}
	other.start(t)

	return other
}

// start primes the cookies and the CSRF token from the login page.
//
// Parameters:
//   - t: The test that primes the browser.
func (b *browser) start(t *testing.T) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", http.NoBody)

	req.Host = b.host

	resp, err := b.app.Test(req, browserTestConfig())
	require.NoError(t, err)

	b.keep(readAnswer(t, resp).cookies)

	require.NotEmpty(t, b.csrfToken, "the login page issued a CSRF token")
}

// submit posts a form carrying the CSRF token.
//
// Parameters:
//   - t: The test that posts the form.
//   - target: Request path.
//   - fields: Form fields beyond the CSRF token, which may be nil.
//
// Returns:
//   - got: The response the router produced.
func (b *browser) submit(t *testing.T, target string, fields url.Values) answer {
	t.Helper()

	form := url.Values{}
	maps.Copy(form, fields)
	form.Set("_csrf", b.csrfToken)

	return b.do(t, http.MethodPost, target, form)
}

// browserTestConfig is how long the browser waits for one response.
//
// Returns:
//   - config: The Fiber test configuration.
func browserTestConfig() fiber.TestConfig {
	return fiber.TestConfig{Timeout: browserTimeout, FailOnTimeout: true}
}

// readAnswer drains a response and closes it.
//
// Parameters:
//   - t: The test that read the response.
//   - resp: The response to observe.
//
// Returns:
//   - got: Everything the test observes about the response.
func readAnswer(t *testing.T, resp *http.Response) answer {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return answer{
		status:      resp.StatusCode,
		location:    resp.Header.Get(fiber.HeaderLocation),
		contentType: resp.Header.Get(fiber.HeaderContentType),
		body:        string(body),
		cookies:     resp.Cookies(),
	}
}

// newBrowser builds the router over a throwaway database.
//
// Parameters:
//   - t: The test that owns the router.
//
// Returns:
//   - client: A browser wired to a freshly built router.
func newBrowser(t *testing.T) *browser {
	t.Helper()

	db := testRouterDatabase(t)
	plexTV := routerPlexTV(t)

	bind := identity.NewBinding("outtake", "test-client", time.Hour)
	t.Cleanup(bind.Stop)

	return &browser{
		app:       New(testRouterDepsWith(t, db, bind, plexTV.URL)),
		db:        db,
		cookies:   nil,
		csrfToken: "",
		host:      routerHost,
		bind:      bind,
		plexURL:   plexTV.URL,
	}
}

// testRouterDatabase opens a migrated SQLite database inside a temp directory.
//
// Parameters:
//   - t: The test that owns the database.
//
// Returns:
//   - db: A database handle closed when the test finishes.
func testRouterDatabase(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New(filepath.Join(t.TempDir(), "outtake.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// testRouterDeps builds the smallest dependency set New accepts.
//
// The queue is created but never started, so it holds no jobs and runs no
// workers.
//
// Parameters:
//   - t: The test that owns the dependencies.
//   - db: Database the middleware and handlers read through.
//
// Returns:
//   - deps: Dependencies wired to real but idle collaborators.
func testRouterDeps(t *testing.T, db *database.DB) Deps {
	t.Helper()

	return testRouterDepsWith(t, db, nil, routerPlexTV(t).URL)
}

// testRouterDepsWith builds the router's dependencies around a server binding.
//
// Parameters:
//   - t: The test that owns the dependencies.
//   - db: The database.
//   - bind: The server binding, which may be nil.
//   - plexURL: Where plex.tv requests are aimed.
//
// Returns:
//   - deps: Dependencies for New.
func testRouterDepsWith(
	t *testing.T,
	db *database.DB,
	bind *identity.Binding,
	plexURL string,
) Deps {
	t.Helper()

	var selected identity.ServerBinding

	if bind != nil {
		selected = bind
	}

	cfg := &config.Config{
		ListenAddr:  "127.0.0.1:8080",
		LogLevel:    "info",
		Env:         "test",
		SessionPoll: 10 * time.Second,
	}

	return Deps{
		Cfg:      cfg,
		DB:       db,
		Queue:    queue.NewQueue(1, nil),
		Blob:     &blob.Storage{},
		Sessions: database.NewSessionStore(db),
		Auth: identity.New(
			"outtake", "test-client", "http://localhost", db, selected,
			identity.WithPlexURL(plexURL),
		),
		Sources: library.NewMediaSource(cfg, selected, nil),
	}
}

// routerPlexTV stands in for plex.tv.
//
// ownerToken belongs to account 42 and otherToken to account 7, and every
// other token is refused. The owner's resources list no server, so its
// sign-in binds nothing. The other account's list machine-1 shared with it,
// the server member binds.
//
// Parameters:
//   - t: The test that owns the stand-in.
//
// Returns:
//   - server: The running plex.tv stand-in.
func routerPlexTV(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/user", func(writer http.ResponseWriter, request *http.Request) {
		switch request.Header.Get("X-Plex-Token") {
		case ownerToken:
			_, _ = writer.Write([]byte(`{"id": 42, "username": "owner"}`))
		case otherToken:
			_, _ = writer.Write([]byte(`{"id": 7, "username": "other"}`))
		default:
			writer.WriteHeader(http.StatusUnauthorized)
		}
	})
	mux.HandleFunc(
		"/library/metadata/{id}",
		func(writer http.ResponseWriter, request *http.Request) {
			if request.PathValue("id") != sharedTitle ||
				request.Header.Get("X-Plex-Token") != memberServerToken {

				writer.WriteHeader(http.StatusNotFound)

				return
			}

			writer.Header().Set("Content-Type", "application/json")

			_, _ = writer.Write([]byte(
				`{"MediaContainer":{"Metadata":[{"ratingKey":"501","title":"Shared","type":"movie"}]}}`,
			))
		},
	)
	mux.HandleFunc("/api/resources", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Plex-Token") != otherToken {
			_, _ = writer.Write([]byte(`<MediaContainer></MediaContainer>`))

			return
		}

		_, _ = writer.Write([]byte(`<MediaContainer>
	<Device name="Attic" owned="0" clientIdentifier="machine-1" provides="server" accessToken="member-server-token">
		<Connection protocol="http" address="127.0.0.1" port="1" local="1"/>
	</Device>
</MediaContainer>`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

// ownedTargets fills a route that names a clip or a preview by id.
//
// Parameters:
//   - route: A route table row.
//   - clipID: A clip the owner made.
//   - previewID: A preview the owner asked for.
//
// Returns:
//   - target: The route naming the owner's record.
//   - absent: The route naming a record of the same kind that does not exist.
//   - ok: False for a route that names no clip or preview.
//
//nolint:nonamedreturns // Same-type returns need names.
func ownedTargets(route routerRoute, clipID, previewID string) (target, absent string, ok bool) {
	if !strings.Contains(route.path, ":id") {
		return "", "", false
	}

	switch {
	case strings.Contains(route.path, "preview"):
		missing := clippreview.OwnedID(42, strings.Repeat("b", 64))

		return strings.Replace(route.path, ":id", previewID, 1),
			strings.Replace(route.path, ":id", missing, 1), true
	case strings.HasPrefix(route.path, "/clips/") || strings.HasPrefix(route.path, "/api/clips/"):
		return strings.Replace(route.path, ":id", clipID, 1),
			strings.Replace(route.path, ":id", "no-such-clip", 1), true
	default:
		return "", "", false
	}
}
