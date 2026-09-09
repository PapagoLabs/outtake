// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/storage"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/plex/binding"
	authapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/auth"
	clipapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/clip"
	mediaapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/media"
	"github.com/PapagoLabs/outtake/internal/web/handlers/html"
	htmlthumb "github.com/PapagoLabs/outtake/internal/web/handlers/html/thumb"
	sharederror "github.com/PapagoLabs/outtake/internal/web/handlers/shared/error"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
)

// New constructs the Fiber application and registers routes.
//
// Parameters:
//   - cfg: Application configuration.
//   - db: Database handle.
//   - jobQueue: In-process clip job queue.
//   - store: Blob storage backend for clip artifacts.
//   - bind: Live Plex server binding and session monitor.
//   - plexProduct: Typed string argument for New.
//   - plexClientID: Plex client id.
//
// Returns:
//   - app: The Fiber application and registers routes.
func New(
	cfg *config.Config,
	db *database.DB,
	jobQueue *queue.Queue,
	store storage.Blob,
	bind *binding.Binding,
	plexProduct, plexClientID string,
) *fiber.App {
	clipHandler := clipapi.NewClipHandler(
		jobQueue,
		store,
		db,
		cfg,
		bind,
		plexProduct,
		plexClientID,
	)
	mediaHandler := mediaapi.NewMediaHandler(plexProduct, plexClientID, bind)
	authHandler := authapi.NewAuthHandler(plexProduct, plexClientID, cfg.PublicURL(), db, bind)
	htmlHandler := html.NewHTMLHandler(jobQueue, db, bind, cfg, plexProduct, plexClientID)
	thumbHandler := htmlthumb.NewThumbHandler(store, bind, plexProduct, plexClientID)

	app := fiber.New(fiber.Config{
		ErrorHandler: sharederror.PageError,
	})
	app.Use(recover.New())
	app.Use(middleware.RequestLogger())
	app.Use(helmet.New(HelmetConfig()))
	app.Use(session.New(SessionConfig()))
	app.Use(csrf.New(CSRFConfig(cfg)))
	app.Use(middleware.BindCSRFToken())
	app.Use(middleware.RestoreToken(db))
	app.Use("/assets", static.New("assets", StaticConfig()))

	guard := middleware.AuthGuard(cfg.Env)
	MountPages(app, guard, htmlHandler, thumbHandler)
	MountAPI(app, guard, clipHandler, mediaHandler, authHandler)

	return app
}
