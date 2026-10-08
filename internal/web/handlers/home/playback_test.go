// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"testing"

	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/home/mocks"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// routes.PathItemPrefix + ":" + routes.ParamID + "/playback" is the route template the live playback panel is served on.
// playingSessions builds sessions playing the given media items.
//
// Parameters:
//   - sessions: One media id and offset per session, in order.
//
// Returns:
//   - live: The Plex sessions the panel will read.
func playingSessions(sessions ...struct {
	mediaID string
	offset  float64
},
) []plex.Session {
	live := make([]plex.Session, 0, len(sessions))

	for _, session := range sessions {
		live = append(live, plex.Session{
			ID: session.mediaID,
			MediaItem: plex.MediaItem{
				ID:   session.mediaID,
				Type: "movie",
			},
			ViewOffset: session.offset,
		})
	}

	return live
}

// getPlayback serves one live playback panel request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func getPlayback(t *testing.T, handler *Handler) pageAnswer {
	t.Helper()

	app := fiber.New()
	app.Get(routes.PathItemPrefix+":"+routes.ParamID+"/playback", handler.Playback)

	return serve(t, app, routes.PathItemPrefix+"42/playback", true)
}

// playbackAuth builds an authentication whose cache holds the given sessions.
//
// Parameters:
//   - t: The test the authentication belongs to.
//   - sessions: Sessions the panel should read.
//
// Returns:
//   - auth: The authentication under test.
func playbackAuth(t *testing.T, sessions []plex.Session) *mocks.MockPlexAuth {
	t.Helper()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().TokenRejected().Return(false).Maybe()
	auth.EXPECT().Sessions().Return(sessions)

	return auth
}

func TestPlaybackReportsTheLivePosition(t *testing.T) {
	t.Parallel()

	auth := playbackAuth(t, playingSessions(struct {
		mediaID string
		offset  float64
	}{"42", 900}))

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getPlayback(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Plex is at",
		"the live position is what the panel exists to report")
	assertBodyContains(t, answer.body, `data-offset="900.000"`,
		"the mark buttons carry the position they would mark")
	assertBodyContains(t, answer.body, "00:15:00.000",
		"the position is shown as a timecode, not a raw offset")
}

func TestPlaybackAsksForPlaybackWithNothingCached(t *testing.T) {
	t.Parallel()

	auth := playbackAuth(t, nil)

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getPlayback(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Play this item in Plex",
		"with nothing playing the panel says so")
	assertBodyOmits(t, answer.body, "Plex is at",
		"a panel that reported a position with nothing playing would be lying")
}

func TestPlaybackSkipsASessionOnAnotherItem(t *testing.T) {
	t.Parallel()

	auth := playbackAuth(t, playingSessions(struct {
		mediaID string
		offset  float64
	}{"some-other-item", 900}))

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getPlayback(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Play this item in Plex",
		"a session on another item says nothing about this one")
}

func TestPlaybackStopsAtTheFirstSessionOnTheItem(t *testing.T) {
	t.Parallel()

	auth := playbackAuth(t, playingSessions(
		struct {
			mediaID string
			offset  float64
		}{"42", 30},
		struct {
			mediaID string
			offset  float64
		}{"42", 60},
	))

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getPlayback(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `data-offset="30.000"`,
		"the first session on the item is the one the panel reports")
	assertBodyOmits(t, answer.body, `data-offset="60.000"`,
		"the search stops at the first match rather than reporting the last")
}

func TestPlaybackCarriesTheSessionTitle(t *testing.T) {
	t.Parallel()

	auth := playbackAuth(t, []plex.Session{{
		ID:         "session-42",
		MediaItem:  plex.MediaItem{ID: "42", Type: "movie"},
		Title:      "Movie",
		ViewOffset: 1,
	}})

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getPlayback(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `data-offset="1.000"`,
		"a short offset still reaches the mark buttons")
}
