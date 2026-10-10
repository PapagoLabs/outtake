// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
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
	assertBodyContains(t, answer.body, "Plex is playing at",
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
	assertBodyContains(t, answer.body, "Play this title in Plex",
		"with nothing playing the panel says so")
	assertBodyOmits(t, answer.body, "Plex is playing at",
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
	assertBodyContains(t, answer.body, "Play this title in Plex",
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

// TestPlaybackSaysWhenPlexIsPaused covers the paused panel: it says so, and
// drops the advice to pause, since a paused position is already exact.
func TestPlaybackSaysWhenPlexIsPaused(t *testing.T) {
	t.Parallel()

	auth := playbackAuth(t, []plex.Session{{
		ID:         "session-42",
		MediaItem:  plex.MediaItem{ID: "42", Type: "movie"},
		ViewOffset: 90,
		State:      plex.StatePaused,
	}})

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getPlayback(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Plex is paused at", "a paused position is labeled")
	assertBodyOmits(t, answer.body, "Pause in Plex", "a paused client needs no advice to pause")
	assertBodyContains(t, answer.body,
		`data-position-url="/media/item/42/position?session=session-42"`,
		"the mark buttons read the shown session's position fresh when clicked")
}

// TestPlaybackAdvisesPausingWhilePlaying covers a playing session, whose
// position is only as fresh as the client's last report to Plex.
func TestPlaybackAdvisesPausingWhilePlaying(t *testing.T) {
	t.Parallel()

	auth := playbackAuth(t, []plex.Session{{
		ID:         "session-42",
		MediaItem:  plex.MediaItem{ID: "42", Type: "movie"},
		ViewOffset: 90,
		State:      plex.StatePlaying,
	}})

	handler, _ := pageHandler(t, auth, silentSources(t))

	answer := getPlayback(t, handler)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "Plex is playing at", "a playing position is reported")
	assertBodyContains(t, answer.body, "Pause in Plex for an exact position",
		"a playing position lags the client, so pausing is advised")
}

// getPosition serves one fresh position request for media item 42.
//
// Parameters:
//   - t: The test the request belongs to.
//   - auth: Plex authentication the handler reads.
//   - sessionID: The session the request names, empty to name none.
//
// Returns:
//   - position: The decoded answer.
//   - status: The response status.
func getPosition(
	t *testing.T,
	auth *mocks.MockPlexAuth,
	sessionID string,
) (api.PlaybackPosition, int) {
	t.Helper()

	handler, _ := pageHandler(t, auth, silentSources(t))

	app := fiber.New()
	app.Get(routes.PathItemPrefix+":"+routes.ParamID+"/position", handler.Position)

	target := routes.PathItemPrefix + "42/position"
	if sessionID != "" {
		target += "?" + url.Values{routes.QuerySession: {sessionID}}.Encode()
	}

	answer := serve(t, app, target, false)

	var position api.PlaybackPosition

	if answer.status == fiber.StatusOK {
		require.NoError(t, json.Unmarshal([]byte(answer.body), &position))
	}

	return position, answer.status
}

// TestPositionReadsPlexWhenAsked covers the fresh position the mark buttons
// read: the named session on the item and its play state, and nothing when
// that session is gone, plays another item, or no session is named, even
// while another client plays the item.
func TestPositionReadsPlexWhenAsked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		sessions  []plex.Session
		want      api.PlaybackPosition
	}{
		{
			name:      "paused",
			sessionID: "s1",
			sessions: []plex.Session{{
				ID:         "s1",
				MediaItem:  plex.MediaItem{ID: "42"},
				ViewOffset: 61.5,
				State:      plex.StatePaused,
			}},
			want: api.PlaybackPosition{Playing: true, Paused: true, Offset: 61.5},
		},
		{
			name:      "the named session among others on the item",
			sessionID: "s2",
			sessions: []plex.Session{
				{
					ID:         "s1",
					MediaItem:  plex.MediaItem{ID: "42"},
					ViewOffset: 5,
					State:      plex.StatePaused,
				},
				{
					ID:         "s2",
					MediaItem:  plex.MediaItem{ID: "42"},
					ViewOffset: 12.25,
					State:      plex.StatePlaying,
				},
			},
			want: api.PlaybackPosition{Playing: true, Paused: false, Offset: 12.25},
		},
		{
			name:      "the named session is gone while another plays the item",
			sessionID: "s1",
			sessions: []plex.Session{
				{ID: "s2", MediaItem: plex.MediaItem{ID: "42"}, ViewOffset: 5},
			},
			want: api.PlaybackPosition{Playing: false, Paused: false, Offset: 0},
		},
		{
			name:      "the named session moved to another item",
			sessionID: "s1",
			sessions: []plex.Session{
				{ID: "s1", MediaItem: plex.MediaItem{ID: "7"}, ViewOffset: 5},
			},
			want: api.PlaybackPosition{Playing: false, Paused: false, Offset: 0},
		},
		{
			name:      "no session named",
			sessionID: "",
			sessions: []plex.Session{
				{ID: "s1", MediaItem: plex.MediaItem{ID: "42"}, ViewOffset: 5},
			},
			want: api.PlaybackPosition{Playing: false, Paused: false, Offset: 0},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			auth := mocks.NewMockPlexAuth(t)
			auth.EXPECT().LiveSessions(mock.Anything).Return(test.sessions, nil).Once()

			position, status := getPosition(t, auth, test.sessionID)

			require.Equal(t, fiber.StatusOK, status)
			assert.Equal(t, test.want, position)
		})
	}
}

// TestPositionReportsAPlexItCannotReach covers a failed read: the buttons get
// an error and fall back to the position the panel shows.
func TestPositionReportsAPlexItCannotReach(t *testing.T) {
	t.Parallel()

	auth := mocks.NewMockPlexAuth(t)
	auth.EXPECT().LiveSessions(mock.Anything).Return(nil, plex.ErrUnauthorized).Once()

	_, status := getPosition(t, auth, "s1")

	assert.Equal(t, fiber.StatusBadGateway, status)
}
