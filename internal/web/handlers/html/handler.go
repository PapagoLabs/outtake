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
//
// Parameters:
//   - jobQueue: In-process clip job queue.
//   - db: Database handle.
//   - bind: Live Plex server binding and session monitor.
//   - cfg: Application configuration.
//   - product: Plex X-Plex-Product identifier for API clients.
//   - clientID: Plex X-Plex-Client-Identifier.
//
// Returns:
//   - hTMLHandler: The composed HTML page handlers.
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

// Login delegates to the auth HTML handler.
func (h *HTMLHandler) Login(ctx fiber.Ctx) error { return h.auth.Login(ctx) }

// Dashboard delegates to the dashboard HTML handler.
func (h *HTMLHandler) Dashboard(ctx fiber.Ctx) error {
	return h.dashboard.Dashboard(ctx)
}

// DashboardSessions delegates to the dashboard sessions fragment handler.
func (h *HTMLHandler) DashboardSessions(ctx fiber.Ctx) error {
	return h.dashboard.DashboardSessions(ctx)
}

// Media delegates to the media HTML handler.
func (h *HTMLHandler) Media(ctx fiber.Ctx) error { return h.media.Media(ctx) }

// Playback delegates to the media playback fragment handler.
func (h *HTMLHandler) Playback(ctx fiber.Ctx) error { return h.media.Playback(ctx) }

// MediaItemClips delegates to the media item clips fragment handler.
func (h *HTMLHandler) MediaItemClips(ctx fiber.Ctx) error {
	return h.media.MediaItemClips(ctx)
}

// MediaItem delegates to the media item HTML handler.
func (h *HTMLHandler) MediaItem(ctx fiber.Ctx) error { return h.media.MediaItem(ctx) }

// PreviewFile delegates to the clips preview file handler.
func (h *HTMLHandler) PreviewFile(ctx fiber.Ctx) error { return h.clips.PreviewFile(ctx) }

// NavLibraries delegates to the media nav libraries fragment handler.
func (h *HTMLHandler) NavLibraries(ctx fiber.Ctx) error { return h.media.NavLibraries(ctx) }

// NewClip delegates to the clips create handler.
func (h *HTMLHandler) NewClip(ctx fiber.Ctx) error { return h.clips.NewClip(ctx) }

// ClipFile delegates to the clips file download handler.
func (h *HTMLHandler) ClipFile(ctx fiber.Ctx) error { return h.clips.ClipFile(ctx) }

// ClipRow delegates to the clips row fragment handler.
func (h *HTMLHandler) ClipRow(ctx fiber.Ctx) error { return h.clips.ClipRow(ctx) }

// Clips delegates to the clips list HTML handler.
func (h *HTMLHandler) Clips(ctx fiber.Ctx) error { return h.clips.Clips(ctx) }

// Servers delegates to the servers HTML handler.
func (h *HTMLHandler) Servers(ctx fiber.Ctx) error { return h.servers.Servers(ctx) }

// SelectServer delegates to the select-server HTML handler.
func (h *HTMLHandler) SelectServer(ctx fiber.Ctx) error {
	return h.servers.SelectServer(ctx)
}

// Appearance delegates to the appearance settings HTML handler.
func (h *HTMLHandler) Appearance(ctx fiber.Ctx) error { return h.settings.Appearance(ctx) }

// ClipProfiles delegates to the clip profiles settings HTML handler.
func (h *HTMLHandler) ClipProfiles(ctx fiber.Ctx) error {
	return h.settings.ClipProfiles(ctx)
}

// CreateClipProfile delegates to the create clip profile handler.
func (h *HTMLHandler) CreateClipProfile(ctx fiber.Ctx) error {
	return h.settings.CreateClipProfile(ctx)
}

// SetDefaultClipProfile delegates to the set-default clip profile handler.
func (h *HTMLHandler) SetDefaultClipProfile(ctx fiber.Ctx) error {
	return h.settings.SetDefaultClipProfile(ctx)
}

// DeleteClipProfile delegates to the delete clip profile handler.
func (h *HTMLHandler) DeleteClipProfile(ctx fiber.Ctx) error {
	return h.settings.DeleteClipProfile(ctx)
}

// UpdateClipProfile delegates to the update clip profile handler.
func (h *HTMLHandler) UpdateClipProfile(ctx fiber.Ctx) error {
	return h.settings.UpdateClipProfile(ctx)
}
