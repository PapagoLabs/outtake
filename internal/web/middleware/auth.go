// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3/middleware/session"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// UserLookup resolves the user a session signed in as.
type UserLookup interface {
	// UserRole returns the role of the user behind a Plex account, reporting
	// whether the account belongs to a user.
	UserRole(ctx context.Context, plexUserID int) (string, bool, error)
}

// callerKey is the request local AuthGuard stores the caller under.
type callerKey struct{}

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
// Caller returns.
//
// Parameters:
//   - env: Configured environment. The e2e environment skips the check.
//   - users: Store the signed-in user is resolved against. A nil store refuses
//     every request.
//
// Returns:
//   - handler: Middleware that redirects or answers anonymous requests.
func AuthGuard(env string, users UserLookup) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		if env == config.EnvE2E {
			SetCaller(ctx, e2eCaller)

			return ctx.Next()
		}

		caller, signedIn, err := signedInUser(ctx, users)
		if err != nil {
			return fmt.Errorf("auth guard: %w", err)
		}

		if !signedIn {
			return unauthenticated(ctx)
		}

		SetCaller(ctx, caller)

		return ctx.Next()
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
