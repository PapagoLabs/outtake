// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/catalog"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// Dashboard handles the dashboard page request.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) Dashboard(ctx fiber.Ctx) error {
	stats := catalog.Summarize(catalog.Jobs(ctx.Context(), handler.queue, handler.db))

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.Dashboard(pages.DashboardProps{
			TotalClips:    stats.Total,
			PendingClips:  stats.Pending,
			Completed:     stats.Completed,
			Failed:        stats.Failed,
			Sessions:      view.SessionItems(handler.auth.Sessions()),
			TokenRejected: handler.auth.TokenRejected(),
		}).Render(ctx.Context(), writer)
	})
}

// DashboardSessions renders the live-sessions fragment for HTMX polling.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) DashboardSessions(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.LiveSessions(
			view.SessionItems(handler.auth.Sessions()),
			handler.auth.TokenRejected(),
		).Render(ctx.Context(), writer)
	})
}
