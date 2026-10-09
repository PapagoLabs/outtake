// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/PapagoLabs/outtake/internal/clip/preview"
	profilesettings "github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	plexlib "github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/handlers/auth"
	clips "github.com/PapagoLabs/outtake/internal/web/handlers/clip"
	"github.com/PapagoLabs/outtake/internal/web/handlers/health"
	"github.com/PapagoLabs/outtake/internal/web/handlers/home"
	"github.com/PapagoLabs/outtake/internal/web/handlers/library"
	"github.com/PapagoLabs/outtake/internal/web/handlers/library/media"
	"github.com/PapagoLabs/outtake/internal/web/handlers/library/thumb"
	previews "github.com/PapagoLabs/outtake/internal/web/handlers/preview"
	profiles "github.com/PapagoLabs/outtake/internal/web/handlers/profile"
	"github.com/PapagoLabs/outtake/internal/web/handlers/server"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/respond"
)

// Deps is every dependency the route table and its handlers are built from. The
// composition root fills it in, so this package assembles the HTTP surface
// without knowing which backend sits behind any of it.
type Deps struct {
	// Cfg is the loaded application configuration.
	Cfg *config.Config
	// DB is the clip database the handlers and middleware read through.
	DB *database.DB
	// Queue is the queue the clip handlers submit to.
	Queue *queue.Queue
	// Blob is the blob store the clip and thumbnail handlers use.
	Blob blob.Blob
	// Paths is the local layout of the stored blobs.
	Paths blob.Paths
	// Auth is the Plex authentication service, which owns the selected server.
	Auth *identity.Auth
	// Previews is the preview service the preview routes submit to.
	Previews *preview.Service
	// Sources resolves the media the pages describe.
	Sources *plexlib.MediaSource
	// Sessions persists web sessions. Nil keeps them in memory.
	Sessions fiber.Storage
}

// routerHandlers holds every handler the route table mounts.
type routerHandlers struct {
	clip     *clips.Handler
	preview  *previews.Handler
	media    *media.Handler
	auth     *auth.Handler
	home     *home.Handler
	library  *library.Handler
	server   *server.Handler
	thumb    *thumb.Handler
	profiles *profiles.Handler
	health   *health.Handler
}

// routeClips is the clips collection path.
const routeClips = "/clips"

// readTimeout bounds how long a client may take to send a request.
const readTimeout = 30 * time.Second

// idleTimeout bounds how long an idle keep-alive connection stays open, so
// shutdown does not wait on it.
const idleTimeout = 120 * time.Second

// routeAPI is the prefix every JSON route sits under.
const routeAPI = "/api"

// routeHealth is the health check path inside the API group. Probes reach it
// by IP address or service name, so the host guard lets it through.
const routeHealth = "/healthz"

// routeAssets is the path the static files are served under.
const routeAssets = "/assets"

// Cache-Control values for static files.
const (
	// cacheForever lets a browser keep an asset whose URL names its contents.
	cacheForever = "public, max-age=31536000, immutable"
	// cacheRevalidate makes a browser check back before reusing an asset.
	cacheRevalidate = "no-cache"
)

// New constructs the Fiber application and registers routes.
//
// Parameters:
//   - deps: Every dependency the route table and its handlers are built from.
//
// Returns:
//   - app: The Fiber application with every route registered.
func New(deps Deps) *fiber.App {
	built := newRouterHandlers(deps)

	app := fiber.New(appConfig())
	useEdgeMiddleware(app, deps.Cfg)
	mountSessionless(app, built)
	useSessionMiddleware(app, deps.Cfg, deps.Sessions)

	guard := middleware.AuthGuard(deps.Cfg.Env, deps.DB)
	mountPages(app, guard, built)
	mountAPI(app, guard, built)

	return app
}

// appConfig returns the Fiber application configuration.
//
// Immutable copies every value read off a request, so a string kept past the
// handler, such as a queue tombstone or the bound server, never changes when
// a later request reuses the buffer behind it. There is no write timeout,
// because downloads and ranged video responses stream for as long as they
// need.
//
// Returns:
//   - config: The Fiber application configuration.
func appConfig() fiber.Config {
	return fiber.Config{
		ErrorHandler: respond.PageError,
		Immutable:    true,
		ReadTimeout:  readTimeout,
		IdleTimeout:  idleTimeout,
	}
}

// newRouterHandlers builds every handler the route table mounts.
//
// Parameters:
//   - deps: Every dependency the route table and its handlers are built from.
//
// Returns:
//   - built: The handlers New mounts.
func newRouterHandlers(deps Deps) routerHandlers {
	// One authentication service is shared, because the login routes and the
	// server picker select the same Plex credentials and the same server.
	plexAuth := deps.Auth

	return routerHandlers{
		clip: clips.New(
			deps.Queue,
			deps.Blob,
			deps.Paths,
			deps.DB,
			deps.Cfg,
			deps.Sources,
		),
		media: media.New(plexAuth),
		auth:  auth.New(plexAuth),
		home: home.New(
			deps.Queue,
			deps.DB,
			plexAuth,
			deps.Cfg,
			deps.Sources,
		),
		library: library.New(
			deps.Queue,
			deps.DB,
			plexAuth,
			deps.Cfg,
			deps.Sources,
			deps.Blob,
		),
		server: server.New(
			deps.Queue,
			deps.DB,
			plexAuth,
			deps.Cfg,
			deps.Sources,
		),
		thumb: thumb.New(
			deps.Blob,
			deps.Paths,
			plexAuth,
		),
		preview: previews.New(
			deps.Previews,
			deps.Cfg,
			deps.DB,
			deps.Sources,
		),
		profiles: profiles.New(profilesettings.New(deps.DB)),
		health:   health.New(),
	}
}

// useEdgeMiddleware installs the middleware every request passes, including
// the ones that never touch a session.
//
// Parameters:
//   - app: The application to install onto.
//   - cfg: Configuration supplying the allowed hosts.
func useEdgeMiddleware(app *fiber.App, cfg *config.Config) {
	app.Use(recover.New())
	app.Use(middleware.RequestLogger())
	app.Use(helmet.New(helmetConfig()))
	app.Use(middleware.HostGuard(hostAllowlist(cfg), routeAPI+routeHealth))
}

// mountSessionless registers the routes that answer before the session
// middleware runs, so assets and health probes never create a session.
//
// Parameters:
//   - app: The Fiber application to register on.
//   - built: Every handler the route table mounts.
func mountSessionless(app *fiber.App, built routerHandlers) {
	app.Use(routeAssets, static.New(".", staticConfig()))
	app.Get(routeAPI+routeHealth, built.health.Health)
}

// useSessionMiddleware installs the session and the CSRF protection bound to
// it.
//
// Parameters:
//   - app: The application to install onto.
//   - cfg: Configuration supplying the cookie settings.
//   - storage: Where sessions persist, or nil to keep them in memory.
func useSessionMiddleware(app *fiber.App, cfg *config.Config, storage fiber.Storage) {
	sessions, store := session.NewWithStore(sessionConfig(cfg, storage))

	app.Use(sessions)
	app.Use(csrf.New(csrfConfig(cfg, store)))
	app.Use(middleware.BindCSRFToken())
}

// mountPages registers HTML routes.
//
// Parameters:
//   - app: The Fiber application to register on.
//   - guard: The authentication middleware protecting every page.
//   - built: Every handler the route table mounts.
func mountPages(app *fiber.App, guard fiber.Handler, built routerHandlers) {
	homeHandler := built.home
	libraryHandler := built.library
	serverHandler := built.server
	clipHandler := built.clip
	previewHandler := built.preview
	thumbHandler := built.thumb
	profilesHandler := built.profiles

	app.Get("/login", homeHandler.Login)
	app.Get("/", guard, homeHandler.Dashboard)
	app.Get("/dashboard/sessions", guard, homeHandler.DashboardSessions)
	app.Get("/media", guard, libraryHandler.Media)
	app.Get("/media/item/:id/playback", guard, homeHandler.Playback)
	app.Get("/media/item/:id/position", guard, homeHandler.Position)
	app.Get("/media/item/:id/clips", guard, libraryHandler.MediaItemClips)
	app.Get("/media/item/:id", guard, libraryHandler.MediaItem)
	app.Get("/previews/:id", guard, previewHandler.PreviewFile)
	app.Get("/nav/libraries", guard, libraryHandler.NavLibraries)
	app.Get("/thumbs", guard, thumbHandler.Get)
	app.Get("/clips/new", guard, libraryHandler.NewClip)
	app.Get("/clips/:id/file", guard, clipHandler.ClipFile)
	app.Get("/clips/:id/row", guard, clipHandler.ClipRow)
	app.Get(routeClips, guard, clipHandler.Clips)
	app.Get("/servers", guard, serverHandler.Servers)
	app.Post("/servers", guard, serverHandler.SelectServer)
	app.Post("/servers/forget", guard, serverHandler.ForgetServer)
	app.Get("/settings/appearance", guard, homeHandler.Appearance)
	app.Get("/settings/profiles", guard, profilesHandler.ClipProfiles)
	app.Post("/settings/profiles", guard, profilesHandler.CreateClipProfile)
	app.Post("/settings/profiles/:id/default", guard, profilesHandler.SetDefaultClipProfile)
	app.Post("/settings/profiles/:id/delete", guard, profilesHandler.DeleteClipProfile)
	app.Post("/settings/profiles/:id", guard, profilesHandler.UpdateClipProfile)
}

// mountAPI registers JSON API routes.
//
// Parameters:
//   - app: The Fiber application to register on.
//   - guard: The authentication middleware protecting the clip and media routes.
//   - built: Every handler the route table mounts.
func mountAPI(app *fiber.App, guard fiber.Handler, built routerHandlers) {
	clipHandler := built.clip
	previewHandler := built.preview
	mediaHandler := built.media
	authHandler := built.auth

	api := app.Group(routeAPI)
	api.Post(routeClips, guard, clipHandler.Create)
	api.Post("/clips/preview", guard, previewHandler.Preview)
	api.Get("/clips/preview/:id", guard, previewHandler.PreviewStatus)
	api.Delete("/clips/preview/:id", guard, previewHandler.CancelPreview)
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
	api.Post("/auth/logout", authHandler.Logout)
}
