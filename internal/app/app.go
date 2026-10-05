// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package app provides the application composition root and lifecycle management.
// It wires all dependencies and provides the main entry point for running the server.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web"
)

// App holds the application state and dependencies.
type App struct {
	cfg    *config.Config
	router *fiber.App
	queue  *queue.Queue
	db     *database.DB
	bind   *identity.Binding
	//nolint:containedctx // a service holding its own lifetime context, not a request one.
	ctx  context.Context
	stop context.CancelFunc
}

const (
	// shutdownTimeout is how long a graceful shutdown waits.
	shutdownTimeout = 10 * time.Second
)

// New creates a new App with all dependencies initialized.
//
// Parameters:
//   - cfg: The loaded configuration every dependency is built from.
//
// Returns:
//   - app: The wired application.
//   - error: Non-nil when a dependency fails to initialize.
func New(cfg *config.Config) (*App, error) {
	logging.InitFromConfig(cfg)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGTERM,
	)

	db, err := database.NewFromConfig(cfg)
	if err != nil {
		stop()

		return nil, fmt.Errorf("init database: %w", err)
	}

	store, err := initStorage(cfg)
	if err != nil {
		stop()

		_ = db.Close()

		return nil, fmt.Errorf("init storage: %w", err)
	}

	runner := ffmpeg.NewExecFFmpeg(cfg.FFmpegPath, cfg.FFprobePath)

	plexAuth, bind := plexIdentity(cfg, db)

	jobQueue := startQueue(ctx, cfg, db, runner, store.blob)

	previews, sources := renderServices(cfg, store, bind, runner)

	return &App{
		ctx:  ctx,
		stop: stop,
		cfg:  cfg,
		router: web.New(web.Deps{
			Cfg:      cfg,
			DB:       db,
			Queue:    jobQueue,
			Blob:     store.blob,
			Paths:    store.paths,
			Auth:     plexAuth,
			Previews: previews,
			Sources:  sources,
		}),
		queue: jobQueue,
		db:    db,
		bind:  bind,
	}, nil
}

// plexIdentity derives this installation's Plex identity, restores the server it
// is bound to, and builds the authentication service the handlers share.
//
// Parameters:
//   - cfg: Application configuration carrying the client id and Plex credentials.
//   - db: Database the last selected server is restored from.
//
// Returns:
//   - auth: Plex authentication service for this installation.
//   - bind: The server selection that service resolves media against.
func plexIdentity(cfg *config.Config, db *database.DB) (*identity.Auth, *identity.Binding) {
	product := "outtake"

	clientID := cfg.PlexClientID
	if clientID == "" {
		clientID = identity.GenerateClientID()
	}

	bind := identity.NewBinding(
		product,
		clientID,
		cfg.SessionPoll,
	)
	restoreBinding(cfg, db, bind)

	return identity.New(product, clientID, cfg.PublicURL(), db, bind), bind
}

// renderServices builds the services that turn a request into a render.
//
// Parameters:
//   - cfg: Application configuration.
//   - store: Storage the previews are written through.
//   - bind: Plex server selection the sources resolve against.
//   - ffmpeg: Runner used for detection and preview renders.
//
// Returns:
//   - previews: Preview service the preview routes submit to.
//   - sources: Resolver for the media a page describes.
func renderServices(
	cfg *config.Config,
	store blobStore,
	bind *identity.Binding,
	runner *ffmpeg.ExecFFmpeg,
) (*preview.Service, *library.MediaSource) {
	previews := preview.New(cfg.MaxConcurrentPreviews, store.blob, store.paths, runner)

	return previews, library.NewMediaSource(cfg, bind, runner)
}

// Close cleans up application resources.
func (app *App) Close() {
	app.stop()

	app.queue.Stop()
	app.bind.Stop()

	err := app.db.Close()
	if err != nil {
		log.Error().Err(err).Msg("error closing database")
	}
}

// Run starts the application server and blocks until shutdown.
//
// Returns:
//   - error: Non-nil when the listener fails for a reason other than shutdown.
func (app *App) Run() error {
	ctx := app.ctx

	go func() {
		<-ctx.Done()
		log.Info().Msg("shutdown signal received")

		err := app.router.ShutdownWithTimeout(shutdownTimeout)
		if err != nil {
			log.Error().Err(err).Msg("error during shutdown")
		}
	}()

	err := app.router.Listen(app.cfg.ListenAddr)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info().Msg("server closed")

			return nil
		}

		return fmt.Errorf("listen: %w", err)
	}

	return nil
}

// Test processes an HTTP request through the Fiber app and returns the response.
//
// Parameters:
//   - req: The request to process.
//
// Returns:
//   - resp: The response the router produced.
//   - error: Non-nil when the router cannot process the request.
func (app *App) Test(req *http.Request) (*http.Response, error) {
	resp, err := app.router.Test(req)
	if err != nil {
		return nil, fmt.Errorf("test: %w", err)
	}

	return resp, nil
}
