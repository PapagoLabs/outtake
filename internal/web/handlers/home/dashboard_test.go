// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/handlers/home/mocks"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// dashboardApp mounts the dashboard routes on a fresh app.
//
// Parameters:
//   - handler: The handler under test.
//
// Returns:
//   - app: The app the routes are mounted on.
func dashboardApp(handler *Handler) *fiber.App {
	app := fiber.New()
	app.Get("/", handler.Dashboard)
	app.Get("/dashboard/sessions", handler.DashboardSessions)

	return app
}

// getDashboard serves one dashboard request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - target: Request target, including any query string.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func getDashboard(t *testing.T, handler *Handler, target string) pageAnswer {
	t.Helper()

	return serve(t, dashboardApp(handler), target, false)
}

// getDashboardSessions serves one live sessions fragment request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func getDashboardSessions(t *testing.T, handler *Handler) pageAnswer {
	t.Helper()

	return serve(t, dashboardApp(handler), "/dashboard/sessions", true)
}

// storeDashboardClip persists a clip for the dashboard to summarize.
//
// Parameters:
//   - t: The test the clip belongs to.
//   - db: Database the clip is stored in.
//   - id: Identifier the clip is stored under.
//   - status: Status the clip is in.
func storeDashboardClip(
	t *testing.T,
	db *database.DB,
	id string,
	status clip.Status,
) {
	t.Helper()

	job := testClipJob(id, clip.TypeClip)

	job.Status = status
	require.NoError(t, db.SaveClip(t.Context(), job))
}

func TestDashboardSummarisesTheStoredClips(t *testing.T) {
	t.Parallel()

	handler, db := pageHandler(t, offlineAuth(t), silentSources(t))
	storeDashboardClip(t, db, "queued", clip.StatusPending)
	storeDashboardClip(t, db, "busy", clip.StatusProcessing)
	storeDashboardClip(t, db, "done", clip.StatusCompleted)
	storeDashboardClip(t, db, "broken", clip.StatusFailed)

	answer := getDashboard(t, handler, "/")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Dashboard",
		"the dashboard is the page an install opens on")
	assertBodyContains(t, answer.body,
		`text-yellow-500">2</p>`,
		"a queued clip and a clip mid-render are both still pending work")
	assertBodyContains(t, answer.body,
		`text-green-500">1</p>`,
		"the finished clip is counted separately from the pending ones")
	assertBodyContains(t, answer.body,
		`text-muted-foreground">Failed</p>`,
		"a failed clip is counted on its own so the user can retry it")
}

func TestDashboardRendersWithNothingStored(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := getDashboard(t, handler, "/")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Dashboard",
		"an empty install still gets the dashboard, not an error")
}

func TestDashboardFollowsTheCarriedView(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := getDashboard(t, handler, "/"+"?view=live")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Live",
		"the view the dashboard link carried has to be the one it renders")
}

func TestDashboardSessionsServesThePollFragment(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().TokenRejected().Return(false).Maybe()
	auth.EXPECT().Sessions().Return([]plex.Session{{
		ID:         "session-42",
		MediaItem:  plex.MediaItem{ID: "42", Title: "Movie", Type: "movie"},
		Title:      "Movie",
		ViewOffset: 900,
		Duration:   7200,
	}})

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getDashboardSessions(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Clip now",
		"a live session has to offer a way back into the item it is playing")
	assertBodyContains(t, answer.body, routes.ItemURL("42",
		url.Values{routes.QueryStart: {"900"}}),
		"the resume link starts where Plex is")
}

func TestDashboardSessionsIsEmptyWithNothingPlaying(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().TokenRejected().Return(false).Maybe()
	auth.EXPECT().Sessions().Return(nil)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getDashboardSessions(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyOmits(t, answer.body, "Clip now",
		"a poll that finds nothing must swap the rows out rather than leave stale ones")
}

func TestDashboardSessionsNeedsNoServerBound(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().TokenRejected().Return(false).Maybe()
	auth.EXPECT().Sessions().Return(nil)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getDashboardSessions(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status,
		"the dashboard polls every few seconds, so an unbound install still renders")
}

// TestDashboardSessionsExplainsARefusedToken covers the live sessions panel
// while the bound server refuses its token: it says why no sessions show.
func TestDashboardSessionsExplainsARefusedToken(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().TokenRejected().Return(true)
	auth.EXPECT().Sessions().Return(nil)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getDashboardSessions(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "data-token-rejected",
		"the panel explains a refused token instead of showing nothing")
	assertBodyContains(t, answer.body, `href="/servers"`, "and links to where it is fixed")
	assertBodyOmits(t, answer.body, "No one is playing",
		"the sessions could not be read, so the panel does not claim nothing is playing")
}
