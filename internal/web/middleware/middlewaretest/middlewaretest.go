// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package middlewaretest lets handler tests make requests as a signed-in
// caller without a session, recording the caller the way the auth guard does.
package middlewaretest

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
)

// OwnerID is the plex.tv account id the owner signs in as.
const OwnerID = 42

// MemberID is the plex.tv account id a member signs in as.
const MemberID = 7

// AsCaller records a caller for every request that passes it.
//
// Parameters:
//   - caller: The user each request is made by.
//
// Returns:
//   - handler: Middleware that records the caller and continues the chain.
func AsCaller(caller identity.User) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		middleware.SetCaller(ctx, caller)

		return ctx.Next()
	}
}

// AsOwner records the installation's owner as every request's caller.
//
// Returns:
//   - handler: Middleware that records the owner and continues the chain.
func AsOwner() fiber.Handler {
	return AsCaller(identity.User{PlexID: OwnerID, Username: "owner", Role: identity.RoleOwner})
}

// AsMember records a member as every request's caller.
//
// Returns:
//   - handler: Middleware that records the member and continues the chain.
func AsMember() fiber.Handler {
	return AsCaller(identity.User{PlexID: MemberID, Username: "member", Role: identity.RoleMember})
}
