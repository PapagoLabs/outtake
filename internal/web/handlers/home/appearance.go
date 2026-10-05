// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// Appearance renders the color palette settings page.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (*Handler) Appearance(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Appearance().Render(ctx.Context(), writer)
	})
}
