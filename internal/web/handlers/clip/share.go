// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"errors"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// Share shares a clip with the installation's other users.
//
// They may then play it, download it, and follow its status when Plex shares
// its source title with them, but not change it.
//
// Parameters:
//   - ctx: Request context carrying the clip id route parameter.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) Share(ctx fiber.Ctx) error {
	//nolint:wrapcheck // setShared writes the response and wraps its own failures.
	return handler.setShared(ctx, true)
}

// Unshare makes a shared clip private again.
//
// Parameters:
//   - ctx: Request context carrying the clip id route parameter.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) Unshare(ctx fiber.Ctx) error {
	//nolint:wrapcheck // setShared writes the response and wraps its own failures.
	return handler.setShared(ctx, false)
}

// setShared shares a clip the caller may change, or makes it private.
//
// An htmx request gets the clip's card, a form post is sent back to the page
// it came from, and an API caller gets the clip.
//
// Parameters:
//   - ctx: Request context carrying the clip id route parameter.
//   - shared: Whether the clip is shared.
//
// Returns:
//   - err: Non-nil when the response cannot be written.
func (handler *Handler) setShared(ctx fiber.Ctx, shared bool) error {
	id := ctx.Params(routes.ParamID)
	notFound := newRejection(
		fiber.StatusNotFound,
		api.NotFound,
		view.NewNotice(respond.NotFoundMessage),
	)

	if handler.lookupJob(ctx.Context(), middleware.ClipScope(ctx), id) == nil {
		//nolint:wrapcheck // The helper writes the response itself.
		return respondWithUpdateFailure(ctx, notFound)
	}

	job, err := catalog.Share(ctx.Context(), handler.clipQueue, handler.db, id, shared)
	if errors.Is(err, catalog.ErrClipNotFound) {
		//nolint:wrapcheck // The helper writes the response itself.
		return respondWithUpdateFailure(ctx, notFound)
	}

	if err != nil {
		//nolint:wrapcheck // The helper writes the response itself.
		return respondWithUpdateFailure(ctx, newRejection(
			fiber.StatusInternalServerError,
			api.PersistFailed,
			respond.Fail(ctx, err),
		))
	}

	if !respond.IsHTMXRequest(ctx) && !respond.IsFormRequest(ctx) {
		return respond.WriteJSON(ctx, fiber.StatusOK, catalog.ClipResponse(job))
	}

	//nolint:wrapcheck // The helper writes the response itself.
	return handler.respondWithClipCard(ctx, job)
}
