// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package storage

import (
	"context"
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// FileDeleter deletes one stored file, which is all the page needs of the
// storage.
type FileDeleter interface {
	// DeleteFile deletes the object identified by path.
	DeleteFile(path string) error
}

// PreviewClearer deletes the finished previews.
type PreviewClearer interface {
	// ClearFinished deletes every finished preview and its file.
	ClearFinished(ctx context.Context) (int, error)
}

// Handler serves the storage settings page.
type Handler struct {
	// store is the storage the thumbnails are deleted from.
	store FileDeleter
	// previews clears the finished previews.
	previews PreviewClearer
	// paths is the local layout the page measures.
	paths blob.Paths
	// s3 reports that clips are kept in an S3 bucket.
	s3 bool
}

// New creates the storage handler.
//
// Parameters:
//   - store: The storage the thumbnails are deleted from.
//   - paths: The local layout the page measures.
//   - previews: Clears the finished previews, nil when no previews are
//     rendered.
//   - s3: Whether clips are kept in an S3 bucket.
//
// Returns:
//   - handler: The storage handler.
func New(store FileDeleter, paths blob.Paths, previews PreviewClearer, s3 bool) *Handler {
	return &Handler{store: store, previews: previews, paths: paths, s3: s3}
}

// ClearPreviews deletes the finished previews and returns to the page.
//
// Parameters:
//   - ctx: The request.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) ClearPreviews(ctx fiber.Ctx) error {
	if handler.previews == nil {
		return respond.RedirectTo(ctx, routes.PathSettingsStorage)
	}

	_, err := handler.previews.ClearFinished(ctx.Context())
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))
	}

	return respond.RedirectTo(ctx, routes.PathSettingsStorage)
}

// ClearThumbnails empties the poster cache and returns to the page.
//
// Parameters:
//   - ctx: The request.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) ClearThumbnails(ctx fiber.Ctx) error {
	_, err := blob.ClearThumbnails(ctx.Context(), handler.store, handler.paths)
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))
	}

	return respond.RedirectTo(ctx, routes.PathSettingsStorage)
}

// Page renders what the local storage holds.
//
// A folder that cannot be read is reported in the banner, and the page shows
// what could be measured.
//
// Parameters:
//   - ctx: The request.
//
// Returns:
//   - err: Non-nil when the page cannot be written.
func (handler *Handler) Page(ctx fiber.Ctx) error {
	usage, err := handler.paths.Usage()
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))
	}

	free, total, known := blob.FreeSpace(handler.paths.BasePath())

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Storage(pages.StorageProps{
			Usage:       usage,
			Free:        free,
			Total:       total,
			VolumeKnown: known,
			S3:          handler.s3,
		}).Render(ctx.Context(), writer)
	})
}
