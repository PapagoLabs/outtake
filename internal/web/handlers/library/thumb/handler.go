// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package thumb serves the Plex thumbnail cache proxy, which answers a poster
// request from the local cache and otherwise fetches it from the selected server.
package thumb

import (
	"fmt"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// ServerSelection resolves the Plex client a thumbnail request is served from.
type ServerSelection interface {
	// Client builds a Plex client for the selected server.
	Client() (*plex.Client, plex.Server, bool)
}

// Handler proxies and caches Plex thumbnails.
type Handler struct {
	store    blob.Blob
	paths    blob.Paths
	selected ServerSelection
}

// New creates a thumbnail handler.
//
// Parameters:
//   - store: Blob store holding the thumbnail cache.
//   - paths: Path layout for cached thumbnails.
//   - selected: Plex server selection the fetch is resolved against.
//
// Returns:
//   - handler: A thumbnail handler wired to the supplied collaborators.
func New(
	store blob.Blob,
	paths blob.Paths,
	selected ServerSelection,
) *Handler {
	// Bundle thumbnail storage and Plex binding.
	return &Handler{
		store:    store,
		paths:    paths,
		selected: selected,
	}
}

// Get serves a cached thumbnail or fetches it from the selected Plex server.
//
// Parameters:
//   - ctx: Request context carrying the path query parameter.
//
// Returns:
//   - err: Wrapped send error, or nil on success.
func (handler *Handler) Get(ctx fiber.Ctx) error {
	thumbPath := ctx.Query("path")
	if !plex.ValidThumbPath(thumbPath) {
		return respond.SendStatusCode(ctx, fiber.StatusBadRequest)
	}

	client, server, ok := handler.selected.Client()
	if !ok {
		return respond.SendStatusCode(ctx, fiber.StatusBadRequest)
	}

	cacheID := library.CacheID(plex.SelectionKey(server), thumbPath)
	cached := handler.paths.ThumbnailPath(cacheID)

	if handler.store.Ensure(ctx.Context(), cached) == nil {
		return sendCachedThumb(ctx, cached)
	}

	body, contentType, err := client.GetThumb(ctx.Context(), server, thumbPath)
	if err != nil {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	// The cache write leaves the thumbnail on local disk, so it is served from
	// there. A thumbnail the cache could not take is still sent.
	if handler.store.WriteThumbnail(cacheID, body) != nil {
		return sendThumbBytes(ctx, body, contentType)
	}

	return sendCachedThumb(ctx, cached)
}

// sendThumbBytes writes thumbnail bytes when the disk cache cannot be written.
//
// Parameters:
//   - ctx: Request context.
//   - body: Thumbnail bytes from Plex.
//   - contentType: Content type Plex reported for the thumbnail.
//
// Returns:
//   - err: Wrapped send error, or nil on success.
func sendThumbBytes(ctx fiber.Ctx, body []byte, contentType string) error {
	ctx.Set("Cache-Control", library.CacheControl)
	ctx.Set(fiber.HeaderContentType, contentType)

	err := ctx.Send(body)
	if err != nil {
		return fmt.Errorf("send thumb: %w", err)
	}

	return nil
}

// sendCachedThumb writes a thumbnail file with a long-lived cache header.
//
// Parameters:
//   - ctx: Request context.
//   - filePath: Local path of the cached thumbnail.
//
// Returns:
//   - err: Wrapped send error, or nil on success.
func sendCachedThumb(ctx fiber.Ctx, filePath string) error {
	ctx.Set("Cache-Control", library.CacheControl)

	err := ctx.SendFile(filePath)
	if err != nil {
		return fmt.Errorf("send thumb: %w", err)
	}

	return nil
}
