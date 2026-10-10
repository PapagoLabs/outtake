// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"io"

	"github.com/gofiber/fiber/v3/middleware/session"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// Login renders the login page and redirects an authenticated visitor home.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Non-nil when rendering or the redirect fails.
func (*Handler) Login(ctx fiber.Ctx) error {
	if identity.Token(session.FromContext(ctx)) != "" {
		return respond.RedirectTo(ctx, routes.PathRoot)
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Login(pages.LoginProps{
			AuthURL: ctx.Query("authUrl"),
		}).Render(ctx.Context(), writer)
	})
}
