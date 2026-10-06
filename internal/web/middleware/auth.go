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
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// UserLookup resolves the user a session signed in as.
type UserLookup interface {
	// UserRole returns the role of the user behind a Plex account, reporting
	// whether the account belongs to a user.
	UserRole(ctx context.Context, plexUserID int) (string, bool, error)
}

// AuthGuard rejects a request whose session has not signed in as a user this
// installation still recognizes.
//
// The user is looked up on every request, so removing it revokes every
// session it holds at once.
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
			return ctx.Next()
		}

		signedIn, err := signedInUser(ctx, users)
		if err != nil {
			return fmt.Errorf("auth guard: %w", err)
		}

		if !signedIn {
			return unauthenticated(ctx)
		}

		return ctx.Next()
	}
}

// signedInUser reports whether a request's session signed in as a user this
// installation still recognizes, resetting a session whose user is gone.
//
// Parameters:
//   - ctx: Request context carrying the session.
//   - users: Store the user is resolved against, which may be nil.
//
// Returns:
//   - signedIn: True when the session belongs to a stored user.
//   - err: Wrapped error when the user or the reset could not be handled.
func signedInUser(ctx fiber.Ctx, users UserLookup) (bool, error) {
	sess := session.FromContext(ctx)

	plexUserID := identity.UserID(sess)
	if users == nil || plexUserID == 0 || identity.Token(sess) == "" {
		return false, nil
	}

	_, found, err := users.UserRole(ctx.Context(), plexUserID)
	if err != nil {
		return false, fmt.Errorf("resolve session user: %w", err)
	}

	if found {
		return true, nil
	}

	err = identity.Reset(sess)
	if err != nil {
		return false, fmt.Errorf("reset revoked session: %w", err)
	}

	return false, nil
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
