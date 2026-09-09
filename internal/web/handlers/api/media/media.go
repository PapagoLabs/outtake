// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/binding"
	"github.com/PapagoLabs/outtake/internal/web/api"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared"
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
			return shared.WriteJSON(ctx, fiber.StatusOK, []api.SessionResponse{})
		}

		live, err := plexClient.GetSessionsOnServer(ctx.Context(), server)
		if err != nil {
			return shared.WriteJSON(ctx, fiber.StatusOK, []api.SessionResponse{})
		}

		sessions = live
	}

	return shared.WriteJSON(ctx, fiber.StatusOK, shared.SessionResponses(sessions))
}

// Search handles the search media request.
func (handler *MediaHandler) Search(ctx fiber.Ctx) error {
	query := ctx.Query("q")
	if query == "" {
		return shared.WriteError(ctx, fiber.StatusBadRequest, "missing_query", "search query is required")
	}

	plexClient, server, ok := handler.plexClient()
	if !ok {
		return shared.WriteJSON(ctx, fiber.StatusOK, api.MediaListResponse{
			Items: []api.MediaItemResponse{},
			Total: 0,
		})
	}

	items, err := plexClient.SearchOnServer(ctx.Context(), server, query, ctx.Query(shared.QueryLibrary))
	if err != nil {
		return shared.WriteError(ctx, fiber.StatusInternalServerError, "search_failed", err.Error())
	}

	responses := make([]api.MediaItemResponse, 0, len(items))
	for index := range items {
		item := items[index]

		responses = append(responses, api.MediaItemResponse{
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

	return shared.WriteJSON(ctx, fiber.StatusOK, api.MediaListResponse{
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

	return shared.NewBoundClient(handler.product, handler.clientID, server.Token), server, true
}
