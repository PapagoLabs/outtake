// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"context"
	"io"
	"net/url"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/components/playback"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// positionTimeout bounds a fresh position read, so a mark button that cannot
// reach Plex falls back to the panel's position quickly.
const positionTimeout = 3 * time.Second

// Playback renders live Plex playback for a media item. The mark buttons read
// their position from the session the panel shows.
//
// Parameters:
//   - ctx: Request carrying the media item id.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) Playback(ctx fiber.Ctx) error {
	mediaID := ctx.Params(routes.ParamID)
	props := view.Playback{
		Playing:     false,
		Paused:      false,
		ViewOffset:  0,
		Title:       "",
		PositionURL: "",
	}

	if session, found := sessionOn(handler.auth.Sessions(), mediaID); found {
		props.Playing = true
		props.Paused = session.State == plex.StatePaused
		props.ViewOffset = timecode.FromSeconds(session.ViewOffset).Duration()
		props.Title = session.Title
		props.PositionURL = routes.ItemURL(mediaID, nil) + "/position?" +
			url.Values{routes.QuerySession: {session.ID}}.Encode()
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return playback.PlaybackPanel(props).Render(ctx.Context(), writer)
	})
}

// Position reports where one Plex session is in a media item, read from the
// server when asked rather than from the session monitor's last poll. Only the
// named session counts, so a second client playing the same item never
// supplies the position.
//
// Parameters:
//   - ctx: Request carrying the media item id and the session query parameter.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) Position(ctx fiber.Ctx) error {
	readCtx, cancel := context.WithTimeout(ctx.Context(), positionTimeout)
	defer cancel()

	sessions, err := handler.auth.LiveSessions(readCtx)
	if err != nil {
		return respond.WriteError(
			ctx,
			fiber.StatusBadGateway,
			api.PositionFailed,
			"the Plex position could not be read",
		)
	}

	position := api.PlaybackPosition{Playing: false, Paused: false, Offset: 0}

	session, found := sessionByID(
		sessions,
		ctx.Params(routes.ParamID),
		ctx.Query(routes.QuerySession),
	)
	if found {
		position.Playing = true
		position.Paused = session.State == plex.StatePaused
		position.Offset = timecode.FromSeconds(session.ViewOffset).Duration().Seconds()
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, position)
}

// sessionOn finds the first session playing a media item.
//
// Parameters:
//   - sessions: Plex playback sessions.
//   - mediaID: Plex media item id.
//
// Returns:
//   - session: The first session on the item.
//   - found: False when nothing plays the item.
func sessionOn(sessions []plex.Session, mediaID string) (plex.Session, bool) {
	for index := range sessions {
		if sessions[index].MediaItem.ID == mediaID {
			return sessions[index], true
		}
	}

	return plex.Session{}, false
}

// sessionByID finds one session by its id, and only while it plays the media
// item.
//
// Parameters:
//   - sessions: Plex playback sessions.
//   - mediaID: Plex media item id.
//   - sessionID: Plex session id, which never matches when empty.
//
// Returns:
//   - session: The named session.
//   - found: False when the session is gone or plays another item.
func sessionByID(sessions []plex.Session, mediaID, sessionID string) (plex.Session, bool) {
	if sessionID == "" {
		return plex.Session{}, false
	}

	for index := range sessions {
		if sessions[index].ID == sessionID && sessions[index].MediaItem.ID == mediaID {
			return sessions[index], true
		}
	}

	return plex.Session{}, false
}
