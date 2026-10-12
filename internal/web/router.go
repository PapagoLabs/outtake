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
	"github.com/PapagoLabs/outtake/internal/clip/quota"
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
	"github.com/PapagoLabs/outtake/internal/web/handlers/live"
	previews "github.com/PapagoLabs/outtake/internal/web/handlers/preview"
	profiles "github.com/PapagoLabs/outtake/internal/web/handlers/profile"
	"github.com/PapagoLabs/outtake/internal/web/handlers/server"
	"github.com/PapagoLabs/outtake/internal/web/handlers/storage"
	"github.com/PapagoLabs/outtake/internal/web/handlers/users"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// Deps is every dependency the route table and its handlers are built from.
//
// The composition root fills it in, so this package assembles the HTTP surface
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
	live     *live.Handler
	storage  *storage.Handler
	users    *users.Handler
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

// routeHealth is the health check path inside the API group.
//
// Probes reach it by IP address or service name, so the host guard lets it
// through.
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

	wireLimits(built, newLimits(deps))

	app := fiber.New(appConfig())
	useEdgeMiddleware(app, deps.Cfg)
	mountSessionless(app, built)
	useSessionMiddleware(app, deps.Cfg, deps.Sessions)

	var tokens middleware.ServerTokens

	if deps.Auth != nil {
		tokens = deps.Auth
	}

	guard := middleware.AuthGuard(deps.Cfg.Env, deps.DB, tokens)
	mountPages(app, guard, built)
	mountAdminPages(app, guard, built)
	mountAPI(app, guard, built)

	// The server waits for open connections as it shuts down, and a clip
	// stream stays open until it is told to end.
	app.Hooks().OnPreShutdown(func() error {
		built.live.CloseStreams()

		return nil
	})

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

	clipHandler := clips.New(
		deps.Queue,
		deps.Blob,
		deps.Paths,
		deps.DB,
		deps.Cfg,
		deps.Sources,
	)
	homeHandler := home.New(
		deps.Queue,
		deps.DB,
		plexAuth,
		deps.Cfg,
		deps.Sources,
	)
	previewHandler := previews.New(
		deps.Previews,
		deps.Cfg,
		deps.DB,
		deps.Sources,
	)

	return routerHandlers{
		clip:  clipHandler,
		media: media.New(plexAuth),
		auth:  auth.New(plexAuth),
		home:  homeHandler,
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
		preview:  previewHandler,
		profiles: profiles.New(profilesettings.New(deps.DB)),
		health:   health.New(),
		storage:  newStorageHandler(deps),
		users:    users.New(deps.DB, deps.DB),
		live: live.New(
			live.Feed{Name: routes.QueryLiveClips, Start: clipHandler.LiveClips},
			live.Feed{Name: routes.QueryPreview, Start: previewHandler.LivePreview},
			live.Feed{Name: routes.QueryLivePlayback, Start: homeHandler.LivePlayback},
		),
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
//   - sessionStorage: Where sessions persist, or nil to keep them in memory.
func useSessionMiddleware(app *fiber.App, cfg *config.Config, sessionStorage fiber.Storage) {
	sessions, store := session.NewWithStore(sessionConfig(cfg, sessionStorage))

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
	clipHandler := built.clip
	previewHandler := built.preview
	thumbHandler := built.thumb

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
	app.Get("/clips/:id/sdr", guard, clipHandler.ClipSDRFile)
	app.Get("/clips/:id/row", guard, clipHandler.ClipRow)
	app.Get(routeClips, guard, clipHandler.Clips)
	app.Get(routes.PathLive, guard, built.live.Stream)
	app.Get("/settings/appearance", guard, homeHandler.Appearance)
	app.Get(routes.PathPending, guard, built.users.Pending)
}

// mountAdminPages registers the pages and actions that manage the
// installation, which only the owner and admins may use.
//
// Parameters:
//   - app: The Fiber application to register on.
//   - guard: The authentication middleware, which records the caller the
//     admin guard checks.
//   - built: Every handler the route table mounts.
func mountAdminPages(app *fiber.App, guard fiber.Handler, built routerHandlers) {
	admin := middleware.AdminGuard()
	homeHandler := built.home
	serverHandler := built.server
	profilesHandler := built.profiles

	app.Get("/servers", guard, admin, serverHandler.Servers)
	app.Post("/servers", guard, admin, serverHandler.SelectServer)
	app.Post("/servers/forget", guard, admin, serverHandler.ForgetServer)
	app.Get(routes.PathSettingsPreviews, guard, admin, homeHandler.PreviewSettings)
	app.Get("/settings/profiles", guard, admin, profilesHandler.ClipProfiles)
	app.Post("/settings/profiles", guard, admin, profilesHandler.CreateClipProfile)
	app.Post(routes.PathSettingsPreviews, guard, admin, homeHandler.SavePreviewSettings)
	app.Get(routes.PathSettingsStorage, guard, admin, built.storage.Page)
	app.Post(routes.PathClearPreviews, guard, admin, built.storage.ClearPreviews)
	app.Post(routes.PathClearThumbnails, guard, admin, built.storage.ClearThumbnails)
	app.Post("/settings/profiles/:id/default", guard, admin, profilesHandler.SetDefaultClipProfile)
	app.Post("/settings/profiles/:id/delete", guard, admin, profilesHandler.DeleteClipProfile)
	app.Post("/settings/profiles/:id", guard, admin, profilesHandler.UpdateClipProfile)
	app.Get(routes.PathSettingsUsers, guard, admin, built.users.Page)
	app.Post(routes.PathSettingsUsers+"/limits", guard, admin, built.users.SaveGlobalLimits)
	app.Post(routes.PathSettingsUsers+"/:id/approve", guard, admin, built.users.Approve)
	app.Post(routes.PathSettingsUsers+"/:id/reject", guard, admin, built.users.Reject)
	app.Post(routes.PathSettingsUsers+"/:id/remove", guard, admin, built.users.Remove)
	app.Post(routes.PathSettingsUsers+"/:id/role", guard, admin, built.users.SetRole)
	app.Post(routes.PathSettingsUsers+"/:id/limits", guard, admin, built.users.SaveUserLimits)
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

// wireLimits hands the enforcer to every handler that starts a render.
//
// Parameters:
//   - built: The handlers New mounts.
//   - limits: The enforcer.
func wireLimits(built routerHandlers, limits *quota.Enforcer) {
	built.clip.SetLimits(limits)
	built.preview.SetLimits(limits)
}

// newLimits builds the enforcer that admits a member's renders.
//
// A member's concurrent renders count their clips waiting or rendering and their
// previews not yet finished.
//
// Parameters:
//   - deps: Every dependency the route table and its handlers are built from.
//
// Returns:
//   - limits: The enforcer, nil without a database, which refuses members.
func newLimits(deps Deps) *quota.Enforcer {
	if deps.DB == nil {
		return nil
	}

	return quota.New(deps.DB, func(owner int) int {
		active := 0

		if deps.Queue != nil {
			active += deps.Queue.CountActive(owner)
		}

		if deps.Previews != nil {
			active += deps.Previews.Active(owner)
		}

		return active
	})
}

// newStorageHandler builds the storage page, which clears previews only when
// a preview service is wired, so the page never calls a nil service.
//
// Parameters:
//   - deps: Every dependency the route table and its handlers are built from.
//
// Returns:
//   - handler: The storage page's handler.
func newStorageHandler(deps Deps) *storage.Handler {
	var clearer storage.PreviewClearer

	if deps.Previews != nil {
		clearer = deps.Previews
	}

	return storage.New(deps.Blob, deps.Paths, clearer, deps.Cfg.StorageBackend == blob.BackendS3)
}
