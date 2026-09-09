// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package dashboard

import (
	"io"

	fiber "github.com/gofiber/fiber/v3"

	htmldeps "github.com/PapagoLabs/outtake/internal/web/handlers/html/deps"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	"github.com/PapagoLabs/outtake/internal/web/pages/dashboard"
)

// Handler serves dashboard HTML routes.
type Handler struct {
	rt *htmldeps.Runtime
}

// New constructs a dashboard HTML handler.
func New(rt *htmldeps.Runtime) *Handler {
	return &Handler{rt: rt}
}

func (h *Handler) Dashboard(ctx fiber.Ctx) error {
	jobs := h.rt.ListJobs(ctx)
	stats := htmldeps.ClipStats(jobs)

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return dashboard.Dashboard(dashboard.DashboardProps{
			TotalClips:   stats.Total,
			PendingClips: stats.Pending,
			Completed:    stats.Completed,
			Failed:       stats.Failed,
			Sessions:     h.rt.SessionItems(),
		}).Render(ctx.Context(), writer)
	})
}

func (h *Handler) DashboardSessions(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return dashboard.LiveSessions(h.rt.SessionItems()).Render(ctx.Context(), writer)
	})
}
