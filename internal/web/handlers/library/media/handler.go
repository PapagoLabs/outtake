// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package media serves the routes that read Plex media directly: the live
// session lookup and the media search.
package media

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// PlexLookup resolves the Plex client and playback sessions a request reads.
type PlexLookup interface {
	// Client builds a Plex client for the selected server.
	Client() (*plex.Client, plex.Server, bool)

	// Sessions returns the latest cached playback sessions.
	Sessions() []plex.Session
}

// Handler handles the Plex media lookups.
type Handler struct {
	auth PlexLookup
}

// New creates a new media handler.
//
// Parameters:
//   - auth: Plex lookup, which owns the selected server and its sessions.
//
// Returns:
//   - handler: A media handler wired to the supplied collaborators.
func New(auth PlexLookup) *Handler {
	return &Handler{auth: auth}
}

// GetSessions handles the get sessions request.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Response error, or nil on success.
func (handler *Handler) GetSessions(ctx fiber.Ctx) error {
	sessions := handler.auth.Sessions()
	if sessions == nil {
		plexClient, server, ok := handler.auth.Client()
		if !ok {
			return respond.WriteJSON(ctx, fiber.StatusOK, []api.SessionResponse{})
		}

		live, err := plexClient.GetSessionsOnServer(ctx.Context(), server)
		if err != nil {
			return respond.WriteJSON(ctx, fiber.StatusOK, []api.SessionResponse{})
		}

		sessions = live
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, library.SessionResponses(sessions))
}

// Search handles the search media request.
//
// Parameters:
//   - ctx: Request context carrying the q and library query parameters.
//
// Returns:
//   - err: Response error, or nil on success.
func (handler *Handler) Search(ctx fiber.Ctx) error {
	query := ctx.Query("q")
	if query == "" {
		return respond.WriteError(
			ctx,
			fiber.StatusBadRequest,
			api.MissingQuery,
			"Enter something to search for",
		)
	}

	plexClient, server, ok := handler.auth.Client()
	if !ok {
		return respond.WriteJSON(ctx, fiber.StatusOK, api.MediaListResponse{
			Items: []api.MediaItemResponse{},
			Total: 0,
		})
	}

	items, err := plexClient.SearchOnServer(
		ctx.Context(),
		server,
		query,
		ctx.Query(routes.QueryLibrary),
	)
	if err != nil {
		return respond.WriteFailure(
			ctx,
			fiber.StatusInternalServerError,
			api.SearchFailed,
			respond.FailWith(ctx, "Couldn't search Plex", err),
		)
	}

	responses := library.ItemResponses(items)

	return respond.WriteJSON(ctx, fiber.StatusOK, api.MediaListResponse{
		Items: responses,
		Total: len(responses),
	})
}
