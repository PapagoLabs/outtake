// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package app provides the application composition root and lifecycle management.
// It wires all dependencies and provides the main entry point for running the server.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
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

	// clientIDSetting names the persisted Plex client identifier.
	clientIDSetting = "plex_client_id"
)

// errE2EOffLoopback refuses the e2e environment on an address other hosts can
// reach, because that environment skips sign-in.
var errE2EOffLoopback = errors.New("the e2e environment must listen on a loopback address")

// errClientIDUnavailable reports that no random Plex client id could be made.
var errClientIDUnavailable = errors.New("generate plex client id")

// New creates a new App with all dependencies initialized.
//
// Parameters:
//   - cfg: The loaded configuration every dependency is built from.
//
// Returns:
//   - app: The wired application.
//   - error: Non-nil when a dependency fails to initialize.
func New(cfg *config.Config) (*App, error) {
	err := checkEnvironment(cfg)
	if err != nil {
		return nil, fmt.Errorf("check environment: %w", err)
	}

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

	plexAuth, bind, err := plexIdentity(cfg, db)
	if err != nil {
		stop()

		_ = db.Close()

		return nil, fmt.Errorf("init plex identity: %w", err)
	}

	jobQueue := startQueue(ctx, cfg, db, runner, store.blob, store.paths)

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
			Sessions: startSessionStore(ctx, db),
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
//   - db: Database the client id, the owner, and the last selected server live in.
//
// Returns:
//   - auth: Plex authentication service for this installation.
//   - bind: The server selection that service resolves media against.
//   - err: Wrapped error when the client id cannot be read or persisted.
func plexIdentity(
	cfg *config.Config,
	db *database.DB,
) (*identity.Auth, *identity.Binding, error) {
	product := "outtake"

	clientID, err := resolveClientID(context.Background(), cfg, db)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve client id: %w", err)
	}

	bind := identity.NewBinding(
		product,
		clientID,
		cfg.SessionPoll,
	)
	restoreBinding(cfg, db, bind)

	return identity.New(product, clientID, cfg.PublicURL(), db, bind), bind, nil
}

// resolveClientID returns the Plex client identifier this installation
// presents. A configured id wins. Otherwise the first start generates one and
// keeps it, so Plex sees the same device across restarts.
//
// Parameters:
//   - ctx: Lifetime context for the read and the write.
//   - cfg: Application configuration, which may name the id.
//   - db: Database the generated id is kept in.
//
// Returns:
//   - clientID: The identifier to present.
//   - err: Wrapped error when the stored id cannot be read, a new one cannot be
//     generated, or it cannot be persisted.
func resolveClientID(ctx context.Context, cfg *config.Config, db *database.DB) (string, error) {
	if cfg.PlexClientID != "" {
		return cfg.PlexClientID, nil
	}

	stored, found, err := db.Setting(ctx, clientIDSetting)
	if err != nil {
		return "", fmt.Errorf("read client id: %w", err)
	}

	if found && stored != "" {
		return stored, nil
	}

	generated := identity.GenerateClientID()
	if generated == "" {
		return "", errClientIDUnavailable
	}

	err = db.SaveSetting(ctx, clientIDSetting, generated)
	if err != nil {
		return "", fmt.Errorf("save client id: %w", err)
	}

	return generated, nil
}

// checkEnvironment refuses a configuration that would expose the e2e
// environment's skipped sign-in beyond this host.
//
// Parameters:
//   - cfg: Application configuration naming the environment and listen address.
//
// Returns:
//   - err: Non-nil for the e2e environment on a non-loopback address.
func checkEnvironment(cfg *config.Config) error {
	if cfg.Env != config.EnvE2E || isLoopbackListen(cfg.ListenAddr) {
		return nil
	}

	return fmt.Errorf("%w: %s", errE2EOffLoopback, cfg.ListenAddr)
}

// isLoopbackListen reports whether a listen address only accepts local
// connections.
//
// Parameters:
//   - addr: Listen address in host:port form.
//
// Returns:
//   - loopback: True for localhost or a loopback IP. An empty host listens on
//     every interface, so it is not loopback.
func isLoopbackListen(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	if host == "localhost" {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.IsLoopback()
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
