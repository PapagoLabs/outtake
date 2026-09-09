// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/PapagoLabs/outtake/internal/web/middleware"

	htmldeps "github.com/PapagoLabs/outtake/internal/web/handlers/html/deps"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	"github.com/PapagoLabs/outtake/internal/web/pages/auth"
)

// Handler serves auth HTML routes.
type Handler struct {
	rt *htmldeps.Runtime
}

// New constructs a auth HTML handler.
//
// Parameters:
//   - rt: Rt.
//
// Returns:
//   - handler: A auth HTML handler.
func New(rt *htmldeps.Runtime) *Handler {
	return &Handler{rt: rt}
}

// Login renders the login page.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (*Handler) Login(ctx fiber.Ctx) error {
	token := respond.SessionString(session.FromContext(ctx), middleware.SessionKeyToken)
	if token != "" {
		return respond.RedirectTo(ctx, respond.PathRoot)
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return auth.Login(auth.LoginProps{
			AuthURL: ctx.Query("authUrl"),
			Error:   ctx.Query(respond.QueryError),
		}).Render(ctx.Context(), writer)
	})
}

// Media handles the media library page request.
//
// Parameters:
//   - ctx: Request with library browse query and optional HX-Target.
//
