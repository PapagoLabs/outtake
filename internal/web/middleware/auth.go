// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/quota"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// UserLookup resolves the user a session signed in as.
type UserLookup interface {
	// UserRole returns the role of the user behind a Plex account, reporting
	// whether the account belongs to a user.
	UserRole(ctx context.Context, plexUserID int) (string, bool, error)
}

// ServerTokens finds an account's own token for the bound Plex server.
type ServerTokens interface {
	// ServerToken returns the account's token for the bound server, empty
	// when the server is not shared with it, and an error when plex.tv could
	// not be asked.
	ServerToken(ctx context.Context, accessToken string) (string, error)
}

// callerKey is the request local AuthGuard stores the caller under.
type callerKey struct{}

// How often AuthGuard asks plex.tv again for a non-owner's server token.
const (
	// serverTokenRetry is how long a session without one waits between
	// asks, so an account the server is not shared with does not ask on
	// every request.
	serverTokenRetry = time.Minute
	// serverTokenRefresh is how long a held token is kept before it is
	// checked, so a rotated token or a removed share is noticed.
	serverTokenRefresh = 10 * time.Minute
)

// e2eCaller is the caller of every request in the e2e environment, which skips
// sign-in and acts as the owner.
var e2eCaller = identity.User{PlexID: 0, Username: "", Role: identity.RoleOwner}

// AdminGuard refuses a request whose caller may not manage the installation.
//
// It runs after AuthGuard, which records the caller, so a request that reaches
// it without one is refused too.
//
// Returns:
//   - handler: Middleware that answers a refused request with
//     respond.ErrAdminOnly.
func AdminGuard() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		caller, ok := Caller(ctx)
		if !ok || !caller.Manages() {
			return respond.ErrAdminOnly
		}

		return ctx.Next()
	}
}

// AuthGuard rejects a request whose session has not signed in as a user this
// installation still recognizes.
//
// The user is looked up on every request, so removing it revokes every
// session it holds at once. The user is recorded as the request's caller, which
// Caller returns. A caller who does not own the installation reads Plex with
// their own server token, which is asked for again when it is missing or old,
// and whose session ends once the server is no longer shared with them. An
// account waiting for approval reaches only the pending page.
//
// Parameters:
//   - env: Configured environment. The e2e environment skips the check.
//   - users: Store the signed-in user is resolved against. A nil store refuses
//     every request.
//   - tokens: Finds a caller's own server token, or nil to keep the one the
//     session holds.
//
// Returns:
//   - handler: Middleware that redirects or answers anonymous requests.
func AuthGuard(env string, users UserLookup, tokens ServerTokens) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		if env == config.EnvE2E {
			SetCaller(ctx, e2eCaller)

			return ctx.Next()
		}

		return guardRequest(ctx, users, tokens)
	}
}

// Caller returns the user a request is made by.
//
// Parameters:
//   - ctx: Request that passed AuthGuard.
//
// Returns:
//   - caller: The signed-in user, with its stored role.
//   - ok: False when AuthGuard did not run for the request.
func Caller(ctx fiber.Ctx) (identity.User, bool) {
	caller, ok := ctx.Locals(callerKey{}).(identity.User)

	return caller, ok
}

// ClipScope returns whose clips and previews a request's caller may see and
// change.
//
// A request AuthGuard never saw has no caller and reaches nothing.
//
// Parameters:
//   - ctx: Request that passed AuthGuard.
//
// Returns:
//   - scope: The caller's own records, or every record for the owner and
//     admins.
func ClipScope(ctx fiber.Ctx) clip.Scope {
	caller, ok := Caller(ctx)
	if !ok {
		return clip.Scope{Owner: 0, Everyone: false}
	}

	return clip.Scope{Owner: caller.PlexID, Everyone: caller.Manages()}
}

// RenderMember returns the account a request renders as, for its limits.
//
// The owner and admins are exempt. A request without a caller is a member
// with no account, whose renders the limits refuse.
//
// Parameters:
//   - ctx: Request that passed AuthGuard.
//
// Returns:
//   - member: The caller as a render member.
func RenderMember(ctx fiber.Ctx) quota.Member {
	caller, _ := Caller(ctx)

	return quota.Member{ID: caller.PlexID, Exempt: caller.Manages()}
}

// PlexViewer returns the account a request reads the bound Plex server as.
//
// The owner reads it with the bound server's token. Anyone else reads it with
// their own token for the server, stored on the session when they signed in.
// A request without a caller reads nothing.
//
// Parameters:
//   - ctx: Request that passed AuthGuard.
//
// Returns:
//   - viewer: The caller as a Plex viewer.
func PlexViewer(ctx fiber.Ctx) identity.Viewer {
	caller, ok := Caller(ctx)
	if !ok {
		return identity.Viewer{Token: "", PlexID: 0, Owner: false}
	}

	if caller.Role == identity.RoleOwner {
		return identity.OwnerViewer(caller.PlexID)
	}

	return identity.Viewer{
		Token:  identity.ServerToken(session.FromContext(ctx)),
		PlexID: caller.PlexID,
		Owner:  false,
	}
}

// SetCaller records the user a request is made by, which Caller returns.
//
// AuthGuard records every signed-in request's caller. Handler tests record
// one through middlewaretest.
//
// Parameters:
//   - ctx: The request.
//   - caller: The user the request is made by.
func SetCaller(ctx fiber.Ctx, caller identity.User) {
	ctx.Locals(callerKey{}, caller)
	ctx.SetContext(view.ContextWithManager(ctx.Context(), caller.Manages()))
}

// renewServerToken asks plex.tv again for a caller's own server token when the
// session's is missing or old.
//
// When plex.tv no longer lists the server for the account, as after the share
// was removed, the session is reset. When plex.tv cannot be asked, the session
// keeps what it holds and waits before asking again.
//
// Parameters:
//   - ctx: Request carrying the session.
//   - tokens: Finds the caller's token, which may be nil.
//   - caller: The signed-in user.
//
// Returns:
//   - kept: False when the session was reset.
func renewServerToken(ctx fiber.Ctx, tokens ServerTokens, caller identity.User) bool {
	if tokens == nil || caller.Role == identity.RoleOwner {
		return true
	}

	sess := session.FromContext(ctx)
	held := identity.ServerToken(sess)

	if !renewalDue(held, identity.ServerTokenChecked(sess)) {
		return true
	}

	token, err := tokens.ServerToken(ctx.Context(), identity.Token(sess))
	if err != nil {
		log.Warn().Err(err).Msg("failed to renew the signed-in account's token for the plex server")
		identity.SetServerToken(sess, held, time.Now())

		return true
	}

	if token == "" {
		signOutUnshared(sess, caller)

		return false
	}

	identity.SetServerToken(sess, token, time.Now())

	return true
}

// renewalDue reports whether a session's server token is due to be asked for
// again.
//
// Parameters:
//   - held: The token the session holds, empty for none.
//   - checked: When plex.tv was last asked.
//
// Returns:
//   - due: True a minute after the last ask for a missing token, and ten
//     minutes after it for a held one.
func renewalDue(held string, checked time.Time) bool {
	age := time.Since(checked)

	return age >= serverTokenRetry && (held == "" || age >= serverTokenRefresh)
}

// signOutUnshared ends the session of an account the bound server is no
// longer shared with.
//
// Parameters:
//   - sess: The session.
//   - caller: The signed-in user.
func signOutUnshared(sess *session.Middleware, caller identity.User) {
	log.Info().
		Int("plex_user_id", caller.PlexID).
		Msg("signed out an account the plex server is no longer shared with")

	err := identity.Reset(sess)
	if err != nil {
		log.Warn().
			Err(err).
			Msg("failed to reset the session of an account the plex server is no longer shared with")
	}
}

// signedInUser resolves the user a request's session signed in as.
//
// A session whose user is gone is reset.
//
// Parameters:
//   - ctx: Request context carrying the session.
//   - users: Store the user is resolved against, which may be nil.
//
// Returns:
//   - caller: The signed-in user, with its stored role and no username.
//   - signedIn: True when the session belongs to a stored user.
//   - err: Wrapped error when the user or the reset could not be handled.
func signedInUser(ctx fiber.Ctx, users UserLookup) (identity.User, bool, error) {
	sess := session.FromContext(ctx)

	plexUserID := identity.UserID(sess)
	if users == nil || plexUserID == 0 || identity.Token(sess) == "" {
		return identity.User{}, false, nil
	}

	role, found, err := users.UserRole(ctx.Context(), plexUserID)
	if err != nil {
		return identity.User{}, false, fmt.Errorf("resolve session user: %w", err)
	}

	if found {
		return identity.User{PlexID: plexUserID, Username: "", Role: identity.Role(role)}, true, nil
	}

	err = identity.Reset(sess)
	if err != nil {
		return identity.User{}, false, fmt.Errorf("reset revoked session: %w", err)
	}

	return identity.User{}, false, nil
}

// guardRequest admits a request whose session signed in as a user this
// installation still recognizes, as AuthGuard describes.
//
// Parameters:
//   - ctx: The request.
//   - users: Store the signed-in user is resolved against.
//   - tokens: Finds a caller's own server token, which may be nil.
//
// Returns:
//   - err: The wrapped failure to resolve the user, or the response's.
func guardRequest(ctx fiber.Ctx, users UserLookup, tokens ServerTokens) error {
	caller, signedIn, err := signedInUser(ctx, users)
	if err != nil {
		return fmt.Errorf("auth guard: %w", err)
	}

	if !signedIn || !renewServerToken(ctx, tokens, caller) {
		//nolint:wrapcheck // The refusal is written as the response.
		return unauthenticated(ctx)
	}

	SetCaller(ctx, caller)

	if caller.Role == identity.RolePending && ctx.Path() != routes.PathPending {
		//nolint:wrapcheck // The refusal is written as the response.
		return awaitingApproval(ctx)
	}

	//nolint:wrapcheck // The rest of the chain answers the request.
	return ctx.Next()
}

// awaitingApproval refuses a request from an account an admin has not
// approved.
//
// Parameters:
//   - ctx: Request from the pending account.
//
// Returns:
//   - err: The write or redirect failure, or nil once the response is sent.
func awaitingApproval(ctx fiber.Ctx) error {
	if strings.HasPrefix(ctx.Path(), "/api/") {
		err := ctx.Status(fiber.StatusForbidden).JSON(api.ErrorResponse{
			Error:   api.Pending,
			Message: "an admin has not approved this account yet",
		})
		if err != nil {
			return fmt.Errorf("write pending: %w", err)
		}

		return nil
	}

	err := ctx.Redirect().To(routes.PathPending)
	if err != nil {
		return fmt.Errorf("redirect pending: %w", err)
	}

	return nil
}

// unauthenticated rejects an anonymous request.
//
// Parameters:
//   - ctx: Request context for the anonymous request.
//
// Returns:
//   - err: The write or redirect failure, or nil once the response is sent.
func unauthenticated(ctx fiber.Ctx) error {
	if strings.HasPrefix(ctx.Path(), "/api/") {
		err := ctx.Status(fiber.StatusUnauthorized).JSON(api.ErrorResponse{
			Error:   "unauthorized",
			Message: "authentication required",
		})
		if err != nil {
			return fmt.Errorf("write unauthorized: %w", err)
		}

		return nil
	}

	err := ctx.Redirect().To("/login")
	if err != nil {
		return fmt.Errorf("redirect login: %w", err)
	}

	return nil
}
