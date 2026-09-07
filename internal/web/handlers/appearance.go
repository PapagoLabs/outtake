// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/pages"
)

// Appearance renders the color palette settings page.
func (*HTMLHandler) Appearance(ctx fiber.Ctx) error {
	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.Appearance().Render(ctx.Context(), writer)
	})
}
