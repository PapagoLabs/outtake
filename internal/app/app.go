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

	"github.com/PapagoLabs/outtake/internal/app/wire"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/storage"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/plex/binding"
	authapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/auth"
	"github.com/PapagoLabs/outtake/internal/web/server"
)

// App holds the application state and dependencies.
type App struct {
	cfg    *config.Config
	router *fiber.App
	queue  *queue.Queue
	db     *database.DB
	bind   *binding.Binding
}

const shutdownTimeout = 10 * time.Second

// New creates a new App with all dependencies initialized.
//
// Parameters:
//   - cfg: Application configuration.
//
// Returns:
//   - app: A new App with all dependencies initialized.
//   - err: Non-nil when database or storage initialization fails.
func New(cfg *config.Config) (*App, error) {
	db, err := database.NewFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	store, err := storage.NewFromConfig(cfg)
	if err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("init storage: %w", err)
	}

	ffmpeg := media.NewExecFFmpeg(cfg.FFmpegPath, cfg.FFprobePath)
	plexProduct := "outtake"
	plexClientID := cfg.PlexClientID
	if plexClientID == "" {
		plexClientID = authapi.GenerateClientID()
	}

	bind := binding.New(plexProduct, plexClientID, time.Duration(cfg.SessionPollSec)*time.Second)
	wire.RestoreBinding(cfg, db, bind)

	jobQueue := wire.StartQueue(cfg, db, ffmpeg, store)

	router := server.New(cfg, db, jobQueue, store, bind, plexProduct, plexClientID)

	return &App{
		cfg:    cfg,
		router: router,
		queue:  jobQueue,
		db:     db,
		bind:   bind,
	}, nil
}

// Close cleans up application resources.
func (app *App) Close() {
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
//   - err: Non-nil when the HTTP server fails to listen.
func (app *App) Run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

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
func (app *App) Test(req *http.Request) (*http.Response, error) {
	resp, err := app.router.Test(req)
	if err != nil {
		return nil, fmt.Errorf("test: %w", err)
	}

	return resp, nil
}
