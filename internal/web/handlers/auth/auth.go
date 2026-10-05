// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"fmt"
	"io"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// Handler adapts the Plex PIN login lifecycle to HTTP.
type Handler struct {
	auth *identity.Auth
}

const (
	// statusWaiting is shown while a PIN is outstanding.
	statusWaiting = "Waiting for Plex authorization..."

	// statusAuthed is shown after PIN authorization succeeds.
	statusAuthed = "Authenticated! Redirecting..."

	// persistTokenMsg is logged when storing the Plex token fails.
	persistTokenMsg = "failed to persist token"

	// msgPlexTokenRequired is shown when the login form is posted empty.
	msgPlexTokenRequired = "Plex token is required"

	// msgInvalidPlexToken is shown when Plex refuses the token that was typed.
	msgInvalidPlexToken = "Plex rejected that token. Check it and try again."
)

// New creates a new auth handler.
//
// Parameters:
//   - auth: Plex authentication service the routes drive.
//
// Returns:
//   - handler: An auth handler wired to the supplied collaborators.
func New(auth *identity.Auth) *Handler {
	return &Handler{
		auth: auth,
	}
}

// Callback completes PIN authorization.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Redirect or send error, or nil on success.
func (handler *Handler) Callback(ctx fiber.Ctx) error {
	sess := session.FromContext(ctx)
	pinID, pinCode := identity.StoredPIN(sess)
	if pinID == 0 {
		return respond.RedirectTo(ctx, respond.PathWithError(routes.PathLogin, "No PIN session"))
	}

	accessToken, err := handler.auth.PollPIN(ctx.Context(), pinID, pinCode)
	if err != nil {
		log.Error().Err(err).Msg("failed to poll PIN")

		return respond.RedirectTo(
			ctx,
			respond.PathWithError(routes.PathLogin, "PIN not yet authorized"),
		)
	}

	identity.ClearStoredPIN(sess)
	handler.finishAuth(ctx, sess, accessToken)

	return sendAuthComplete(ctx, handler.postAuthPath())
}

// Login starts PIN auth or accepts a manual token.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Redirect or response error, or nil on success.
func (handler *Handler) Login(ctx fiber.Ctx) error {
	sess := session.FromContext(ctx)

	accessToken := ctx.FormValue("token")
	if accessToken != "" {
		//nolint:wrapcheck // The helper wraps its own failure with what it was doing.
		return handler.acceptEnteredToken(ctx, accessToken)
	}

	if identity.Token(sess) != "" {
		return respond.RedirectTo(ctx, routes.PathRoot)
	}

	if respond.IsFormRequest(ctx) {
		return respond.RedirectTo(
			ctx,
			respond.PathWithError(routes.PathLogin, msgPlexTokenRequired),
		)
	}

	pin, err := handler.auth.BeginPIN(ctx.Context())
	if err != nil {
		return respond.WriteError(ctx, fiber.StatusBadGateway, api.PINFailed, err.Error())
	}

	identity.SetStoredPIN(sess, pin.ID, pin.Code)

	return respond.WriteJSON(ctx, fiber.StatusOK, fiber.Map{"authUrl": pin.URL})
}

// Logout clears the session and persisted Plex credentials.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Response or redirect error, or nil on success.
func (handler *Handler) Logout(ctx fiber.Ctx) error {
	err := identity.Reset(session.FromContext(ctx))
	if err != nil {
		return respond.WriteError(
			ctx,
			fiber.StatusInternalServerError,
			api.LogoutFailed,
			err.Error(),
		)
	}

	err = handler.auth.Logout(ctx.Context())
	if err != nil {
		log.Warn().Err(err).Msg("failed to clear stored plex credentials")

		return respond.WriteError(
			ctx,
			fiber.StatusInternalServerError,
			api.LogoutFailed,
			err.Error(),
		)
	}

	return respond.RedirectTo(ctx, routes.PathLogin)
}

// Status polls PIN authorization for HTMX.
//
// Parameters:
//   - ctx: Request context, whose HX-Redirect header is set on success.
//
// Returns:
//   - err: Write error, or nil on success.
func (handler *Handler) Status(ctx fiber.Ctx) error {
	sess := session.FromContext(ctx)
	if identity.Token(sess) != "" {
		ctx.Set("HX-Redirect", routes.PathRoot)

		return respond.SendText(ctx, statusAuthed)
	}

	pinID, pinCode := identity.StoredPIN(sess)
	if pinID == 0 {
		return respond.SendText(ctx, statusWaiting)
	}

	accessToken, err := handler.auth.PollPIN(ctx.Context(), pinID, pinCode)
	if err != nil {
		return respond.SendText(ctx, statusWaiting)
	}

	identity.ClearStoredPIN(sess)
	handler.finishAuth(ctx, sess, accessToken)

	ctx.Set("HX-Redirect", handler.postAuthPath())

	return respond.SendText(ctx, statusAuthed)
}

// acceptEnteredToken authenticates a token the user typed, which only takes hold
// when Plex accepts it.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token from the form.
//
// Returns:
//   - err: Redirect error, or nil on success.
func (handler *Handler) acceptEnteredToken(ctx fiber.Ctx, accessToken string) error {
	userID, err := handler.auth.Login(ctx.Context(), accessToken)
	if err != nil {
		log.Warn().Err(err).Msg("rejected entered plex token")

		return wrapAuth(
			respond.RedirectTo(
				ctx,
				respond.PathWithError(routes.PathLogin, msgInvalidPlexToken),
			),
			"reject token",
		)
	}

	sess := session.FromContext(ctx)
	identity.SetToken(sess, accessToken)
	identity.SetUserID(sess, userID)

	return wrapAuth(respond.RedirectTo(ctx, handler.postAuthPath()), "finish auth")
}

// finishAuth records the authenticated token on the session and completes the
// login in the domain. A token that cannot be persisted still authenticates the
// session, so the failure is logged rather than returned.
//
// Parameters:
//   - ctx: Request context.
//   - sess: Fiber session carrying the login state.
//   - accessToken: Plex access token to authenticate.
func (handler *Handler) finishAuth(
	ctx fiber.Ctx,
	sess *session.Middleware,
	accessToken string,
) {
	identity.SetToken(sess, accessToken)

	userID, err := handler.auth.Accept(ctx.Context(), accessToken)
	if err != nil {
		log.Error().Err(err).Msg(persistTokenMsg)
	}

	identity.SetUserID(sess, userID)
}

// postAuthPath returns the next page after authentication.
//
// Returns:
//   - path: routes.PathServers until a server is selected, then routes.PathRoot.
func (handler *Handler) postAuthPath() string {
	if handler.auth.Bound() {
		return routes.PathRoot
	}

	return routes.PathServers
}

// sendAuthComplete finishes popup or full-page login after Plex authorizes.
//
// Parameters:
//   - ctx: Request context.
//   - next: Page the browser is sent to once the login is complete.
//
// Returns:
//   - err: Wrapped render error, or nil on success.
func sendAuthComplete(ctx fiber.Ctx, next string) error {
	err := respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.AuthComplete(next).Render(ctx.Context(), writer)
	})
	if err != nil {
		return fmt.Errorf("send auth complete: %w", err)
	}

	return nil
}

// wrapAuth wraps a handler error with an operation name.
//
// Parameters:
//   - err: Error returned by the handler, which may be nil.
//   - op: Operation name to prefix the error with.
//
// Returns:
//   - Wrapped error, or nil when err is nil.
func wrapAuth(err error, op string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
