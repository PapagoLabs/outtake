// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/components/nav"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// NavLibraries renders sidebar library links.
//
// Parameters:
//   - ctx: Request with an optional HX-Current-URL header.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) NavLibraries(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		selected := selectedLibraryID(
			ctx.Get(routes.HeaderHXCurrentURL),
			ctx.Query(routes.QueryLibrary),
		)

		return nav.NavLibraries(handler.sidebarLibraries(ctx), selected).
			Render(ctx.Context(), writer)
	})
}

// selectedLibraryID returns the library id from the nav query or the current page URL.
//
// Parameters:
//   - currentURL: Page the navigation was rendered on.
//   - fromQuery: Library id carried by the navigation request.
//
// Returns:
//   - libraryID: The selected library id, or empty when neither source has one.
func selectedLibraryID(currentURL, fromQuery string) string {
	if fromQuery != "" {
		return fromQuery
	}

	return respond.QueryValue(currentURL, routes.QueryLibrary)
}
