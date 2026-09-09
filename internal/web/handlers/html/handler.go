// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package html

import (
	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/config"
	htmlauth "github.com/PapagoLabs/outtake/internal/web/handlers/html/auth"
	htmlclips "github.com/PapagoLabs/outtake/internal/web/handlers/html/clips"
	htmldashboard "github.com/PapagoLabs/outtake/internal/web/handlers/html/dashboard"
	htmldeps "github.com/PapagoLabs/outtake/internal/web/handlers/html/deps"
	htmlmedia "github.com/PapagoLabs/outtake/internal/web/handlers/html/media"
	htmlservers "github.com/PapagoLabs/outtake/internal/web/handlers/html/servers"
	htmlsettings "github.com/PapagoLabs/outtake/internal/web/handlers/html/settings"
)

// HTMLHandler composes feature HTML handlers behind the legacy constructor.
type HTMLHandler struct {
	auth      *htmlauth.Handler
	dashboard *htmldashboard.Handler
	media     *htmlmedia.Handler
	clips     *htmlclips.Handler
	servers   *htmlservers.Handler
	settings  *htmlsettings.Handler
}

// NewHTMLHandler creates the composed HTML page handlers.
func NewHTMLHandler(
	jobQueue htmldeps.ClipJobs,
	db htmldeps.ClipStore,
	bind htmldeps.ServerBinding,
	cfg *config.Config,
	product, clientID string,
) *HTMLHandler {
	rt := &htmldeps.Runtime{
		Deps: htmldeps.Deps{
			Queue:    jobQueue,
			DB:       db,
			Bind:     bind,
			Cfg:      cfg,
			Product:  product,
			ClientID: clientID,
		},
	}

	return &HTMLHandler{
		auth:      htmlauth.New(rt),
		dashboard: htmldashboard.New(rt),
		media:     htmlmedia.New(rt),
		clips:     htmlclips.New(rt),
		servers:   htmlservers.New(rt),
		settings:  htmlsettings.New(rt),
	}
}

func (h *HTMLHandler) Login(ctx fiber.Ctx) error { return h.auth.Login(ctx) }
func (h *HTMLHandler) Dashboard(ctx fiber.Ctx) error {
	return h.dashboard.Dashboard(ctx)
}
func (h *HTMLHandler) DashboardSessions(ctx fiber.Ctx) error {
	return h.dashboard.DashboardSessions(ctx)
}
func (h *HTMLHandler) Media(ctx fiber.Ctx) error { return h.media.Media(ctx) }
func (h *HTMLHandler) Playback(ctx fiber.Ctx) error { return h.media.Playback(ctx) }
func (h *HTMLHandler) MediaItemClips(ctx fiber.Ctx) error {
	return h.media.MediaItemClips(ctx)
}
func (h *HTMLHandler) MediaItem(ctx fiber.Ctx) error { return h.media.MediaItem(ctx) }
func (h *HTMLHandler) PreviewFile(ctx fiber.Ctx) error { return h.clips.PreviewFile(ctx) }
func (h *HTMLHandler) NavLibraries(ctx fiber.Ctx) error { return h.media.NavLibraries(ctx) }
func (h *HTMLHandler) NewClip(ctx fiber.Ctx) error { return h.clips.NewClip(ctx) }
func (h *HTMLHandler) ClipFile(ctx fiber.Ctx) error { return h.clips.ClipFile(ctx) }
func (h *HTMLHandler) ClipRow(ctx fiber.Ctx) error { return h.clips.ClipRow(ctx) }
func (h *HTMLHandler) Clips(ctx fiber.Ctx) error { return h.clips.Clips(ctx) }
func (h *HTMLHandler) Servers(ctx fiber.Ctx) error { return h.servers.Servers(ctx) }
func (h *HTMLHandler) SelectServer(ctx fiber.Ctx) error {
	return h.servers.SelectServer(ctx)
}
func (h *HTMLHandler) Appearance(ctx fiber.Ctx) error { return h.settings.Appearance(ctx) }
func (h *HTMLHandler) ClipProfiles(ctx fiber.Ctx) error {
	return h.settings.ClipProfiles(ctx)
}
func (h *HTMLHandler) CreateClipProfile(ctx fiber.Ctx) error {
	return h.settings.CreateClipProfile(ctx)
}
func (h *HTMLHandler) SetDefaultClipProfile(ctx fiber.Ctx) error {
	return h.settings.SetDefaultClipProfile(ctx)
}
func (h *HTMLHandler) DeleteClipProfile(ctx fiber.Ctx) error {
	return h.settings.DeleteClipProfile(ctx)
}
func (h *HTMLHandler) UpdateClipProfile(ctx fiber.Ctx) error {
	return h.settings.UpdateClipProfile(ctx)
}
