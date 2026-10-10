// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// Handler adapts the Plex PIN login lifecycle to HTTP.
type Handler struct {
	auth *identity.Auth
}

const (
	// statusWaiting is shown while a PIN is outstanding.
	statusWaiting = "Waiting for Plex…"

	// statusAuthed is shown after PIN authorization succeeds.
	statusAuthed = "Login complete. Opening Outtake…"

	// statusRefused is shown when Plex authorized a PIN this installation refuses.
	statusRefused = "Login refused"

	// msgPlexTokenRequired is shown when the login form is posted empty.
	msgPlexTokenRequired = "Paste a Plex token"

	// msgInvalidPlexToken is shown when Plex refuses the token that was typed.
	msgInvalidPlexToken = "Plex rejected that token. Check it and try again."

	// msgNotOwner is shown when a Plex account other than the owner signs in.
	msgNotOwner = "This Outtake belongs to a different Plex account. " +
		"Login with the account that set it up."

	// msgSignInFailed is shown when the login fails for a reason the user
	// cannot fix, with the details behind it.
	msgSignInFailed = "Login failed"

	// msgNoPIN is shown when the Plex popup returns with no login in progress.
	msgNoPIN = "This login expired. Start again from the login page."

	// msgPINNotApproved is shown when the Plex popup returns before Plex
	// approved the login.
	msgPINNotApproved = "Plex hasn't approved this login yet"

	// msgPINUnavailable is shown when Plex cannot be reached to start a login.
	msgPINUnavailable = "Couldn't reach Plex to start the login"

	// msgLogoutFailed is shown when the session cannot be ended.
	msgLogoutFailed = "Couldn't logout"
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
		return sendAuthComplete(ctx, routes.PathLogin, view.NewNotice(msgNoPIN))
	}

	accessToken, err := handler.auth.PollPIN(ctx.Context(), pinID, pinCode)
	if err != nil {
		return sendAuthComplete(
			ctx,
			routes.PathLogin,
			respond.FailWith(ctx, msgPINNotApproved, err),
		)
	}

	identity.ClearStoredPIN(sess)

	err = handler.finishAuth(ctx, sess, accessToken)
	if err != nil {
		log.Warn().Err(err).Msg("refused plex sign-in")

		return sendAuthComplete(ctx, routes.PathLogin, refusal(ctx, err))
	}

	return sendAuthComplete(ctx, handler.postAuthPath(), view.NewNotice(""))
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
		respond.SetFlash(ctx, view.NewNotice(msgPlexTokenRequired))

		return respond.RedirectTo(ctx, routes.PathLogin)
	}

	pin, err := handler.auth.BeginPIN(ctx.Context())
	if err != nil {
		return respond.WriteFailure(
			ctx,
			fiber.StatusBadGateway,
			api.PINFailed,
			respond.FailWith(ctx, msgPINUnavailable, err),
		)
	}

	identity.SetStoredPIN(sess, pin.ID, pin.Code)

	return respond.WriteJSON(ctx, fiber.StatusOK, fiber.Map{
		"authUrl":   pin.URL,
		"expiresIn": int(pin.ExpiresIn / time.Second),
	})
}

// Logout ends this browser's session. The owner, the bound server, and every
// other signed-in browser stay as they are.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Response or redirect error, or nil on success.
func (*Handler) Logout(ctx fiber.Ctx) error {
	err := identity.Reset(session.FromContext(ctx))
	if err != nil {
		return respond.WriteFailure(
			ctx,
			fiber.StatusInternalServerError,
			api.LogoutFailed,
			respond.FailWith(ctx, msgLogoutFailed, err),
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
		ctx.Set(routes.HeaderHXRedirect, routes.PathRoot)

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

	err = handler.finishAuth(ctx, sess, accessToken)
	if err != nil {
		log.Warn().Err(err).Msg("refused plex sign-in")

		respond.SetFlash(ctx, refusal(ctx, err))
		ctx.Set(routes.HeaderHXRedirect, routes.PathLogin)

		return respond.SendText(ctx, statusRefused)
	}

	ctx.Set(routes.HeaderHXRedirect, handler.postAuthPath())

	return respond.SendText(ctx, statusAuthed)
}

// acceptEnteredToken signs in with a token the user typed, which only takes
// hold when Plex vouches for it and this installation accepts the account.
//
// Parameters:
//   - ctx: Request context.
//   - accessToken: Plex access token from the form.
//
// Returns:
//   - err: Redirect error, or nil on success.
func (handler *Handler) acceptEnteredToken(ctx fiber.Ctx, accessToken string) error {
	err := handler.finishAuth(ctx, session.FromContext(ctx), accessToken)
	if err != nil {
		log.Warn().Err(err).Msg("refused entered plex token")

		respond.SetFlash(ctx, refusal(ctx, err))

		return wrapAuth(respond.RedirectTo(ctx, routes.PathLogin), "refuse sign-in")
	}

	return wrapAuth(respond.RedirectTo(ctx, handler.postAuthPath()), "finish auth")
}

// finishAuth signs a Plex token in and, once this installation accepts it,
// carries the user on a fresh session. The session id changes, so an id
// planted in the browser before sign-in never becomes an authenticated one.
//
// Parameters:
//   - ctx: Request context.
//   - sess: Fiber session to carry the user, which may be nil.
//   - accessToken: Plex access token to sign in with.
//
// Returns:
//   - err: The wrapped sign-in or session failure. The session is left
//     anonymous when it is non-nil.
func (handler *Handler) finishAuth(
	ctx fiber.Ctx,
	sess *session.Middleware,
	accessToken string,
) error {
	user, err := handler.auth.SignIn(ctx.Context(), accessToken)
	if err != nil {
		return fmt.Errorf("sign in: %w", err)
	}

	if sess != nil {
		err = sess.Regenerate()
		if err != nil {
			return fmt.Errorf("regenerate session: %w", err)
		}
	}

	identity.SetToken(sess, accessToken)
	identity.SetUserID(sess, user.PlexID)

	return nil
}

// refusalMessage explains a failed sign-in to the user.
//
// Parameters:
//   - err: The sign-in failure.
//
// Returns:
//   - message: What the login page shows.
func refusalMessage(err error) string {
	switch {
	case errors.Is(err, identity.ErrNotAllowed):
		return msgNotOwner
	case errors.Is(err, identity.ErrInvalidToken):
		return msgInvalidPlexToken
	default:
		return msgSignInFailed
	}
}

// refusal is what the login page shows for a refused login. A token Plex
// rejected or an account that is not the owner explains itself. Any other
// failure carries details.
//
// Parameters:
//   - ctx: Request context.
//   - err: Why the login was refused.
//
// Returns:
//   - failure: The message, with details for an unexplained failure.
func refusal(ctx fiber.Ctx, err error) view.Failure {
	if errors.Is(err, identity.ErrNotAllowed) || errors.Is(err, identity.ErrInvalidToken) {
		return view.NewNotice(refusalMessage(err))
	}

	return respond.FailWith(ctx, refusalMessage(err), err)
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

// sendAuthComplete finishes popup or full-page login after Plex answers.
// The page carries no opener isolation, so the popup reaches the login page
// that opened it, and sends that window on to next. A failure is kept in the
// session once the popup page has rendered, so the window that opened the
// popup shows it on next rather than the popup that closes.
//
// Parameters:
//   - ctx: Request context.
//   - next: Page the browser is sent to once the login is complete.
//   - failure: What next shows, empty when the login succeeded.
//
// Returns:
//   - err: Wrapped render error, or nil on success.
func sendAuthComplete(ctx fiber.Ctx, next string, failure view.Failure) error {
	ctx.Set(routes.HeaderOpenerPolicy, routes.OpenerPolicyUnsafeNone)

	err := respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.AuthComplete(next).Render(ctx.Context(), writer)
	})
	if err != nil {
		return fmt.Errorf("send auth complete: %w", err)
	}

	respond.SetFlash(ctx, failure)

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
