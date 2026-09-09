// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	fiber "github.com/gofiber/fiber/v3"

	sharedplex "github.com/PapagoLabs/outtake/internal/web/handlers/shared/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/binding"
)

// MediaHandler handles media-related requests.
type MediaHandler struct {
	product  string
	clientID string
	bind     *binding.Binding
}

// NewMediaHandler creates a new media handler.
func NewMediaHandler(product, clientID string, bind *binding.Binding) *MediaHandler {
	return &MediaHandler{
		product:  product,
		clientID: clientID,
		bind:     bind,
	}
}

// GetSessions handles the get sessions request.
func (handler *MediaHandler) GetSessions(ctx fiber.Ctx) error {
	sessions := handler.bind.Sessions()
	if sessions == nil {
		plexClient, server, ok := handler.plexClient()
		if !ok {
			return respond.WriteJSON(ctx, fiber.StatusOK, []SessionResponse{})
		}

		live, err := plexClient.GetSessionsOnServer(ctx.Context(), server)
		if err != nil {
			return respond.WriteJSON(ctx, fiber.StatusOK, []SessionResponse{})
		}

		sessions = live
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, SessionResponses(sessions))
}

// Search handles the search media request.
func (handler *MediaHandler) Search(ctx fiber.Ctx) error {
	query := ctx.Query("q")
	if query == "" {
		return respond.WriteError(ctx, fiber.StatusBadRequest, "missing_query", "search query is required")
	}

	plexClient, server, ok := handler.plexClient()
	if !ok {
		return respond.WriteJSON(ctx, fiber.StatusOK, MediaListResponse{
			Items: []MediaItemResponse{},
			Total: 0,
		})
	}

	items, err := plexClient.SearchOnServer(ctx.Context(), server, query, ctx.Query(respond.QueryLibrary))
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusInternalServerError, "search_failed", err.Error())
	}

	responses := make([]MediaItemResponse, 0, len(items))
	for index := range items {
		item := items[index]

		responses = append(responses, MediaItemResponse{
			ID:           item.ID,
			Title:        item.DisplayTitle(),
			Type:         item.Type,
			Duration:     item.Duration,
			ThumbPath:    item.ThumbPath,
			LibraryTitle: item.LibraryTitle,
			Year:         item.Year,
			Season:       item.ParentIndex,
			Episode:      item.Index,
			ShowTitle:    item.GrandparentTitle,
		})
	}

	return respond.WriteJSON(ctx, fiber.StatusOK, MediaListResponse{
		Items: responses,
		Total: len(responses),
	})
}

// plexClient returns a client for the selected PMS.
func (handler *MediaHandler) plexClient() (*plex.Client, plex.Server, bool) {
	server, ok := handler.bind.Get()
	if !ok {
		return nil, plex.EmptyServer(), false
	}

	return sharedplex.NewBoundClient(handler.product, handler.clientID, server.Token), server, true
}
