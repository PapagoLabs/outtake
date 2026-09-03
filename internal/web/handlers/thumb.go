// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/binding"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/storage"
)

// ThumbHandler proxies and caches Plex thumbnails.
type ThumbHandler struct {
	store    storage.Blob
	bind     *binding.Binding
	product  string
	clientID string
}

// thumbCacheControl tells browsers to reuse cached thumbnails for a week.
const thumbCacheControl = "public, max-age=604800, immutable"

// NewThumbHandler creates a thumbnail handler.
func NewThumbHandler(
	store storage.Blob,
	bind *binding.Binding,
	product, clientID string,
) *ThumbHandler {
	// Bundle thumbnail storage and Plex binding.
	return &ThumbHandler{
		store:    store,
		bind:     bind,
		product:  product,
		clientID: clientID,
	}
}

// Get serves a cached thumbnail or fetches it from the selected Plex server.
func (handler *ThumbHandler) Get(ctx fiber.Ctx) error {
	path := ctx.Query("path")
	if !plex.ValidThumbPath(path) {
		return sendStatusCode(ctx, fiber.StatusBadRequest)
	}

	cacheID := thumbCacheID(path)
	cached := handler.store.ThumbnailPath(cacheID)
	if handler.store.FileExists(cached) {
		err := handler.store.Get(ctx.Context(), cached)
		if err != nil {
			return fmt.Errorf("get cached thumb: %w", err)
		}

		return sendCachedThumb(ctx, cached)
	}

	err := handler.fetchAndCache(ctx, path, cacheID, cached)
	if err != nil {
		return fmt.Errorf("fetch thumb: %w", err)
	}

	return nil
}

// fetchAndCache downloads a thumbnail from Plex and stores it on disk.
func (handler *ThumbHandler) fetchAndCache(
	ctx fiber.Ctx,
	path, cacheID, cached string,
) error {
	// Resolve the bound server before fetching the thumbnail.
	server, ok := handler.bind.Get()
	if !ok {
		return sendStatusCode(ctx, fiber.StatusBadRequest)
	}

	body, contentType, err := newBoundClient(handler.product, handler.clientID, server.Token).
		GetThumb(ctx.Context(), server, path)
	if err != nil {
		return sendStatusCode(ctx, fiber.StatusNotFound)
	}

	writeErr := handler.store.WriteThumbnail(cacheID, body)
	if writeErr != nil {
		return sendThumbBytes(ctx, body, contentType)
	}

	err = handler.store.Get(ctx.Context(), cached)
	if err != nil {
		return sendThumbBytes(ctx, body, contentType)
	}

	return sendCachedThumb(ctx, cached)
}

// sendThumbBytes writes thumbnail bytes when the disk cache cannot be written.
func sendThumbBytes(ctx fiber.Ctx, body []byte, contentType string) error {
	ctx.Type("jpg")
	ctx.Set("Cache-Control", thumbCacheControl)
	ctx.Set(headerContentType, contentType)

	err := ctx.Send(body)
	if err != nil {
		return fmt.Errorf("send thumb: %w", err)
	}

	return nil
}

// sendCachedThumb writes a thumbnail file with a long-lived cache header.
func sendCachedThumb(ctx fiber.Ctx, path string) error {
	ctx.Set("Cache-Control", thumbCacheControl)

	err := ctx.SendFile(path)
	if err != nil {
		return fmt.Errorf("send thumb: %w", err)
	}

	return nil
}

// thumbCacheID hashes a Plex thumb path into a cache filename.
func thumbCacheID(path string) string {
	sum := sha256.Sum256([]byte(path))

	return hex.EncodeToString(sum[:])
}
