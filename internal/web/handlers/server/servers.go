// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"fmt"
	"io"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// SelectServer persists the chosen Plex server.
//
// Parameters:
//   - ctx: Form post carrying the chosen server.
//
// Returns:
//   - err: Non-nil when the chosen URL cannot be bound.
func (handler *Handler) SelectServer(ctx fiber.Ctx) error {
	err := handler.bindSelectedURL(ctx)
	if err != nil {
		return fmt.Errorf("select server: %w", err)
	}

	return nil
}

// Servers lists discovered Plex servers.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) Servers(ctx fiber.Ctx) error {
	current, _ := handler.auth.Selected()

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Servers(pages.ServersProps{
			Servers: view.ServerItems(handler.discoverServers(ctx), current),
			Error:   ctx.Query(routes.QueryError),
		}).Render(ctx.Context(), writer)
	})
}

// bindSelectedURL binds the Plex server the servers page posted.
//
// Parameters:
//   - ctx: Request carrying the form values and the Plex token.
//
// Returns:
//   - err: Non-nil when the chosen server cannot be bound.
func (handler *Handler) bindSelectedURL(ctx fiber.Ctx) error {
	token := ctx.FormValue("token")
	if token == "" {
		token = identity.Token(session.FromContext(ctx))
	}

	server, ok := handler.selectedServer(ctx, token)
	if !ok {
		return respond.RedirectTo(
			ctx,
			respond.PathWithError(routes.PathServers, "invalid server URL"),
		)
	}

	if name := ctx.FormValue("name"); name != "" {
		server.Name = name
	}

	err := handler.auth.Select(ctx.Context(), server)
	if err != nil {
		log.Warn().Err(err).Msg("failed to persist selected server")

		return respond.RedirectTo(ctx, routes.PathRoot)
	}

	return respond.RedirectTo(ctx, routes.PathRoot)
}

// selectedServer reads the posted server, whether it arrived as one URL or as
// the parts of one.
//
// Parameters:
//   - ctx: Request carrying the form values.
//   - token: Plex access token the server is reached with.
//
// Returns:
//   - server: The chosen server connection.
//   - ok: False when the form names no server that can be built.
func (*Handler) selectedServer(ctx fiber.Ctx, token string) (plex.Server, bool) {
	if rawURL := ctx.FormValue("customUrl"); rawURL != "" {
		return plex.ServerFromURL(rawURL, token)
	}

	return plex.ServerFromParts(
		ctx.FormValue("scheme"),
		ctx.FormValue("address"),
		ctx.FormValue("port"),
		token,
	)
}

// discoverServers lists Plex servers for the session token.
//
// Parameters:
//   - ctx: Request carrying the session token.
//
// Returns:
//   - servers: Discovered servers, or nil when the session is anonymous or Plex
//     cannot be reached.
func (handler *Handler) discoverServers(ctx fiber.Ctx) []plex.Server {
	token := identity.Token(session.FromContext(ctx))
	if token == "" {
		return nil
	}

	servers, err := handler.auth.Discover(ctx.Context(), token)
	if err != nil {
		log.Warn().Err(err).Msg("discover servers failed")

		return nil
	}

	return servers
}
