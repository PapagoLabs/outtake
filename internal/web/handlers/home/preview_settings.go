// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"fmt"
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/playback"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// PreviewSettings renders the preview settings page.
//
// Parameters:
//   - ctx: Request context, whose query may carry a failed save's message.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) PreviewSettings(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.PreviewSettings(pages.PreviewSettingsProps{
			MaxPreviewWidth:  playback.MaxPreviewWidth(ctx.Context(), handler.db),
			MaxPreviewWidths: playback.MaxPreviewWidths(),
		}).Render(ctx.Context(), writer)
	})
}

// SavePreviewSettings stores the preview settings a form posted, and returns
// to the page with the reason when they cannot be saved.
//
// Parameters:
//   - ctx: Request context carrying the form fields.
//
// Returns:
//   - err: Redirect error, or nil on success.
func (handler *Handler) SavePreviewSettings(ctx fiber.Ctx) error {
	err := playback.SaveMaxPreviewWidth(ctx.Context(), handler.db, ctx.FormValue("maxPreviewWidth"))
	if err != nil {
		respond.SetFlash(ctx, respond.Fail(ctx, err))
	}

	err = respond.RedirectTo(ctx, routes.PathSettingsPreviews)
	if err != nil {
		return fmt.Errorf("redirect to preview settings: %w", err)
	}

	return nil
}
