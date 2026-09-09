// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package http

import (
	fiber "github.com/gofiber/fiber/v3"

	healthapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/health"
)

const routeClips = "/clips"

// MountPages registers HTML routes.
func MountPages(
	app *fiber.App,
	guard fiber.Handler,
	htmlHandler HTMLPages,
	thumbHandler ThumbPages,
) {
	app.Get("/login", htmlHandler.Login)
	app.Get("/", guard, htmlHandler.Dashboard)
	app.Get("/dashboard/sessions", guard, htmlHandler.DashboardSessions)
	app.Get("/media", guard, htmlHandler.Media)
	app.Get("/media/item/:id/playback", guard, htmlHandler.Playback)
	app.Get("/media/item/:id/clips", guard, htmlHandler.MediaItemClips)
	app.Get("/media/item/:id", guard, htmlHandler.MediaItem)
	app.Get("/previews/:id", guard, htmlHandler.PreviewFile)
	app.Get("/nav/libraries", guard, htmlHandler.NavLibraries)
	app.Get("/thumbs", guard, thumbHandler.Get)
	app.Get("/clips/new", guard, htmlHandler.NewClip)
	app.Get("/clips/:id/file", guard, htmlHandler.ClipFile)
	app.Get("/clips/:id/row", guard, htmlHandler.ClipRow)
	app.Get(routeClips, guard, htmlHandler.Clips)
	app.Get("/servers", guard, htmlHandler.Servers)
	app.Post("/servers", guard, htmlHandler.SelectServer)
	app.Get("/settings/appearance", guard, htmlHandler.Appearance)
	app.Get("/settings/profiles", guard, htmlHandler.ClipProfiles)
	app.Post("/settings/profiles", guard, htmlHandler.CreateClipProfile)
	app.Post("/settings/profiles/:id/default", guard, htmlHandler.SetDefaultClipProfile)
	app.Post("/settings/profiles/:id/delete", guard, htmlHandler.DeleteClipProfile)
	app.Post("/settings/profiles/:id", guard, htmlHandler.UpdateClipProfile)
}

// MountAPI registers JSON API routes.
func MountAPI(
	app *fiber.App,
	guard fiber.Handler,
	clipHandler ClipAPI,
	mediaHandler MediaAPI,
	authHandler AuthAPI,
) {
	api := app.Group("/api")
	api.Post(routeClips, guard, clipHandler.Create)
	api.Post("/clips/preview", guard, clipHandler.Preview)
	api.Post("/clips/:id/update", guard, clipHandler.Update)
	api.Post("/clips/:id/cancel", guard, clipHandler.Cancel)
	api.Get(routeClips, guard, clipHandler.List)
	api.Get("/clips/:id/status", guard, clipHandler.GetStatus)
	api.Get("/clips/:id/download", guard, clipHandler.Download)
	api.Delete("/clips/:id", guard, clipHandler.Delete)
	api.Get("/media/search", guard, mediaHandler.Search)
	api.Get("/sessions", guard, mediaHandler.GetSessions)
	api.Post("/auth/login", authHandler.Login)
	api.Get("/auth/callback", authHandler.Callback)
	api.Get("/auth/status", authHandler.Status)
	api.Get("/auth/logout", authHandler.Logout)
	api.Get("/healthz", healthapi.NewHealthHandler().Health)
}
