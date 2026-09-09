// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package servers

import (
	"fmt"
	"io"

	fiber "github.com/gofiber/fiber/v3"

	htmldeps "github.com/PapagoLabs/outtake/internal/web/handlers/html/deps"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	"github.com/PapagoLabs/outtake/internal/web/pages/settings"
)

// Handler serves servers HTML routes.
type Handler struct {
	rt *htmldeps.Runtime
}

// New constructs a servers HTML handler.
//
// Parameters:
//   - rt: HTML handler runtime (DB, bind, template deps).
//
// Returns:
//   - handler: A servers HTML handler.
func New(rt *htmldeps.Runtime) *Handler {
	return &Handler{rt: rt}
}

// SelectServer selects the active Plex server and redirects.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: Failure from select server.
func (h *Handler) SelectServer(ctx fiber.Ctx) error {
	rawURL := ctx.FormValue("customUrl")
	if rawURL == "" {
		rawURL = htmldeps.ConnectionURL(
			ctx.FormValue("scheme"),
			ctx.FormValue("address"),
			ctx.FormValue("port"),
		)
	}

	err := h.rt.BindSelectedURL(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("select server: %w", err)
	}

	return nil
}

// Servers renders the Plex server picker page.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: Propagates errors from respond.RenderHTML.
func (h *Handler) Servers(ctx fiber.Ctx) error {
	current, _ := h.rt.Bind.Get()

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return settings.Servers(settings.ServersProps{
			Servers: htmldeps.ToServerItems(h.rt.DiscoverServers(ctx), current),
			Error:   ctx.Query(respond.QueryError),
		}).Render(ctx.Context(), writer)
	})
}
