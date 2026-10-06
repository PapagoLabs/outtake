// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"errors"
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

// ForgetServer drops the bound Plex server so the owner can pick another.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Redirect error, or nil on success.
func (handler *Handler) ForgetServer(ctx fiber.Ctx) error {
	err := handler.auth.ForgetServer(ctx.Context())
	if err != nil {
		log.Warn().Err(err).Msg("failed to forget the selected server")

		return respond.RedirectTo(
			ctx,
			respond.PathWithError(
				routes.PathServers,
				"Outtake could not forget the server. Try again.",
			),
		)
	}

	return respond.RedirectTo(ctx, routes.PathServers)
}

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
	current, bound := handler.auth.Selected()

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Servers(pages.ServersProps{
			Servers: view.ServerItems(handler.discoverServers(ctx), current),
			Error:   ctx.Query(routes.QueryError),
			Bound:   bound,
		}).Render(ctx.Context(), writer)
	})
}

// bindSelectedURL binds the Plex server the servers page posted. The page posts
// a selection key or a typed URL, never a token, and the token comes from Plex.
//
// Parameters:
//   - ctx: Request carrying the form values and the session token.
//
// Returns:
//   - err: Non-nil when the redirect cannot be written.
func (handler *Handler) bindSelectedURL(ctx fiber.Ctx) error {
	server, err := handler.chosenServer(ctx, identity.Token(session.FromContext(ctx)))
	if err != nil {
		log.Warn().Err(err).Msg("refused a server choice")

		return respond.RedirectTo(
			ctx,
			respond.PathWithError(routes.PathServers, choiceRefusal(err)),
		)
	}

	err = handler.auth.Select(ctx.Context(), server)
	if err != nil {
		log.Warn().Err(err).Msg("failed to persist selected server")
	}

	return respond.RedirectTo(ctx, routes.PathRoot)
}

// chosenServer resolves the posted server, whether it arrived as a typed URL or
// as the selection key of a discovered connection.
//
// Parameters:
//   - ctx: Request carrying the form values.
//   - token: Plex access token servers are discovered with.
//
// Returns:
//   - server: The chosen connection, with its token.
//   - err: Why the choice cannot be bound.
func (handler *Handler) chosenServer(ctx fiber.Ctx, token string) (plex.Server, error) {
	if rawURL := ctx.FormValue("customUrl"); rawURL != "" {
		server, err := handler.auth.ChooseCustomURL(ctx.Context(), token, rawURL)
		if err != nil {
			return plex.EmptyServer(), fmt.Errorf("choose custom url: %w", err)
		}

		return server, nil
	}

	server, err := handler.auth.ChooseServer(ctx.Context(), token, ctx.FormValue("server"))
	if err != nil {
		return plex.EmptyServer(), fmt.Errorf("choose server: %w", err)
	}

	return server, nil
}

// choiceRefusal explains a refused server choice.
//
// Parameters:
//   - err: Why the choice was refused.
//
// Returns:
//   - message: What the servers page shows.
func choiceRefusal(err error) string {
	switch {
	case errors.Is(err, identity.ErrInvalidServerURL):
		return "Enter an http or https URL for the Plex server."
	case errors.Is(err, identity.ErrServerUnreachable):
		return "Outtake cannot reach that server. Check the address and that Outtake can connect to it."
	case errors.Is(err, identity.ErrServerNotFound):
		return "That is not a Plex server on your account."
	default:
		return "Outtake could not use that server. Try again."
	}
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
