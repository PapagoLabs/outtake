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
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/binding"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/queue"
	"github.com/PapagoLabs/outtake/internal/storage"
	"github.com/PapagoLabs/outtake/internal/web"
	"github.com/PapagoLabs/outtake/internal/web/handlers"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
)

// App holds the application state and dependencies.
type App struct {
	cfg    *config.Config
	router *fiber.App
	queue  *queue.Queue
	db     *database.DB
	bind   *binding.Binding
}

const (
	// ShutdownTimeout is the maximum time to wait for graceful shutdown.
	shutdownTimeout = 10 * time.Second

	// SessionIdleMinutes is the session idle timeout.
	sessionIdleMinutes = 30

	// SessionAbsoluteHours is the session absolute timeout.
	sessionAbsoluteHours = 24

	// RouteClips is the clips collection path.
	routeClips = "/clips"
)

// errUnknownJobType is returned when a job has an unrecognized type.
var errUnknownJobType = errors.New("unknown job type")

// New creates a new App with all dependencies initialized.
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
		plexClientID = handlers.GenerateClientID()
	}

	bind := binding.New(plexProduct, plexClientID, time.Duration(cfg.SessionPollSec)*time.Second)
	restoreBinding(cfg, db, bind)

	jobQueue := startQueue(cfg, db, ffmpeg, store)

	router := newRouter(cfg, db, jobQueue, store, bind, plexProduct, plexClientID)

	return &App{
		cfg:    cfg,
		router: router,
		queue:  jobQueue,
		db:     db,
		bind:   bind,
	}, nil
}

// newRouter constructs the Fiber application and registers routes.
func newRouter(
	cfg *config.Config,
	db *database.DB,
	jobQueue *queue.Queue,
	store storage.Blob,
	bind *binding.Binding,
	plexProduct, plexClientID string,
) *fiber.App {
	// Register page and API routes.
	clipHandler := handlers.NewClipHandler(
		jobQueue,
		store,
		db,
		cfg,
		bind,
		plexProduct,
		plexClientID,
	)
	mediaHandler := handlers.NewMediaHandler(plexProduct, plexClientID, bind)
	authHandler := handlers.NewAuthHandler(plexProduct, plexClientID, cfg.PublicURL(), db, bind)
	htmlHandler := handlers.NewHTMLHandler(jobQueue, db, bind, cfg, plexProduct, plexClientID)
	thumbHandler := handlers.NewThumbHandler(store, bind, plexProduct, plexClientID)

	app := fiber.New(fiber.Config{
		ErrorHandler: handlers.PageError,
	})
	app.Use(recover.New())
	app.Use(middleware.RequestLogger())
	app.Use(session.New(sessionConfig()))
	app.Use(middleware.RestoreToken(db))
	app.Use("/assets", static.New("assets", staticConfig()))

	guard := middleware.AuthGuard(cfg.Env)
	mountPages(app, guard, htmlHandler, thumbHandler)
	mountAPI(app, guard, clipHandler, mediaHandler, authHandler)

	return app
}

// sessionConfig returns the Fiber session middleware configuration.
func sessionConfig() session.Config {
	return session.Config{
		Storage:           nil,
		Store:             nil,
		Next:              nil,
		ErrorHandler:      nil,
		KeyGenerator:      nil,
		CookieDomain:      "",
		CookiePath:        "",
		CookieSameSite:    "Lax",
		Extractor:         extractors.FromCookie("session_id"),
		IdleTimeout:       sessionIdleMinutes * time.Minute,
		AbsoluteTimeout:   sessionAbsoluteHours * time.Hour,
		CookieSecure:      false,
		CookieHTTPOnly:    true,
		CookieSessionOnly: false,
	}
}

// staticConfig returns the static asset middleware configuration.
func staticConfig() static.Config {
	return static.Config{
		FS:              web.Assets,
		Next:            nil,
		ModifyResponse:  nil,
		NotFoundHandler: nil,
		IndexNames:      []string{"index.html"},
		CacheDuration:   0,
		MaxAge:          0,
		Compress:        false,
		ByteRange:       false,
		Browse:          false,
		Download:        false,
	}
}

// mountPages registers HTML routes.
func mountPages(
	app *fiber.App,
	guard fiber.Handler,
	htmlHandler *handlers.HTMLHandler,
	thumbHandler *handlers.ThumbHandler,
) {
	// Mount HTML page routes.
	app.Get("/login", htmlHandler.Login)
	app.Get("/", guard, htmlHandler.Dashboard)
	app.Get("/dashboard/sessions", guard, htmlHandler.DashboardSessions)
	app.Get("/media", guard, htmlHandler.Media)
	app.Get("/media/item/:id/playback", guard, htmlHandler.Playback)
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
	app.Get("/settings/profiles", guard, htmlHandler.ClipProfiles)
	app.Post("/settings/profiles", guard, htmlHandler.CreateClipProfile)
	app.Post("/settings/profiles/:id/default", guard, htmlHandler.SetDefaultClipProfile)
	app.Post("/settings/profiles/:id/delete", guard, htmlHandler.DeleteClipProfile)
	app.Post("/settings/profiles/:id", guard, htmlHandler.UpdateClipProfile)
}

// mountAPI registers JSON API routes.
func mountAPI(
	app *fiber.App,
	guard fiber.Handler,
	clipHandler *handlers.ClipHandler,
	mediaHandler *handlers.MediaHandler,
	authHandler *handlers.AuthHandler,
) {
	// Mount JSON API routes.
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
	api.Get("/healthz", handlers.NewHealthHandler().Health)
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

// startQueue creates the worker queue and restores persisted jobs.
func startQueue(
	cfg *config.Config,
	db *database.DB,
	ffmpeg media.FFmpeg,
	store storage.Blob,
) *queue.Queue {
	jobQueue := queue.NewQueue(cfg.NumWorkers, func(ctx context.Context, job *queue.Job) error {
		progressCtx := media.WithProgress(ctx, func(percent int) {
			job.Progress = percent
			job.UpdatedAt = time.Now()

			saveErr := db.SaveClip(context.WithoutCancel(ctx), job)
			if saveErr != nil {
				log.Warn().Err(saveErr).Str("job_id", job.ID).Msg("failed to persist clip progress")
			}
		})

		return processJob(progressCtx, job, ffmpeg, db, store)
	})
	jobQueue.SetStatusFunc(func(job *queue.Job) {
		saveErr := db.SaveClip(context.Background(), job)
		if saveErr != nil {
			log.Warn().Err(saveErr).Str("job_id", job.ID).Msg("failed to persist clip status")
		}
	})
	jobQueue.Start()
	restoreJobs(db, jobQueue)

	return jobQueue
}

// restoreBinding loads the selected Plex server from config or the database.
func restoreBinding(cfg *config.Config, db *database.DB, bind *binding.Binding) {
	if server, ok := plex.ServerFromURL(cfg.PlexServerURL, cfg.PlexToken); ok {
		bind.Set(server)

		return
	}

	server, ok, err := db.SelectedServer(context.Background())
	if err != nil {
		log.Warn().Err(err).Msg("failed to load selected server")

		return
	}

	if ok {
		bind.Set(server)
	}
}

// restoreJobs reloads persisted clips into the in-memory queue.
func restoreJobs(db *database.DB, jobQueue *queue.Queue) {
	jobs, err := db.ListClips(context.Background())
	if err != nil {
		log.Warn().Err(err).Msg("failed to restore clips")

		return
	}

	for _, job := range jobs {
		switch job.Status {
		case queue.JobStatusPending, queue.JobStatusProcessing:
			job.Status = queue.JobStatusPending
			job.Error = ""
			jobQueue.Submit(job)
		default:
			jobQueue.Restore(job)
		}
	}
}

// extractJob runs the FFmpeg extract for a clip, GIF, or screenshot job.
func extractJob(
	ctx context.Context,
	job *queue.Job,
	ffmpeg media.FFmpeg,
	db *database.DB,
) error {
	switch job.Type {
	case queue.JobTypeClip:
		err := ffmpeg.ExtractClip(
			ctx,
			job.InputPath,
			job.OutputPath,
			job.StartTime,
			job.Duration,
			clipPreset(ctx, db, job.Quality),
			job.AudioIndex,
			detectJobCrop(ctx, ffmpeg, job),
		)
		if err != nil {
			return fmt.Errorf("extract clip: %w", err)
		}
	case queue.JobTypeGIF:
		err := ffmpeg.ExtractGIF(
			ctx,
			job.InputPath,
			job.OutputPath,
			job.StartTime,
			job.Duration,
			job.Width,
			job.FPS,
		)
		if err != nil {
			return fmt.Errorf("extract gif: %w", err)
		}
	case queue.JobTypeScreenshot:
		err := ffmpeg.ExtractScreenshot(ctx, job.InputPath, job.OutputPath, job.StartTime)
		if err != nil {
			return fmt.Errorf("extract screenshot: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", errUnknownJobType, job.Type)
	}

	return nil
}

// processJob routes a job to the appropriate FFmpeg operation.
func processJob(
	ctx context.Context,
	job *queue.Job,
	ffmpeg media.FFmpeg,
	db *database.DB,
	store storage.Blob,
) error {
	err := extractJob(ctx, job, ffmpeg, db)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	if job.OutputPath == "" {
		return nil
	}

	err = store.Put(ctx, job.OutputPath)
	if err != nil {
		return fmt.Errorf("store output: %w", err)
	}

	return nil
}

// detectJobCrop runs cropdetect when the job requested black-bar trimming.
func detectJobCrop(ctx context.Context, ffmpeg media.FFmpeg, job *queue.Job) media.CropRect {
	if !job.CropBlackBars {
		return media.CropRect{}
	}

	crop, err := ffmpeg.DetectCrop(ctx, job.InputPath, job.StartTime, job.Duration)
	if err != nil {
		return media.CropRect{}
	}

	return crop
}

// clipPreset resolves a stored quality id onto ffmpeg settings.
func clipPreset(ctx context.Context, db *database.DB, quality string) media.QualityPreset {
	if quality == "" {
		return defaultClipPreset(ctx, db)
	}

	return media.ResolvePreset(quality, func(id string) (media.QualityPreset, bool) {
		return lookupClipPreset(ctx, db, id)
	})
}

// defaultClipPreset loads the stored default profile, or the built-in medium preset.
func defaultClipPreset(ctx context.Context, db *database.DB) media.QualityPreset {
	if db == nil {
		return media.QualityPresets[media.ClipQualityMedium]
	}

	profile, err := db.DefaultClipProfile(ctx)
	if err != nil {
		return media.QualityPresets[media.ClipQualityMedium]
	}

	return media.NormalizePreset(profile.QualityPreset())
}

// lookupClipPreset loads one stored profile by id.
func lookupClipPreset(ctx context.Context, db *database.DB, id string) (media.QualityPreset, bool) {
	if db == nil {
		return media.QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}, false
	}

	profile, err := db.GetClipProfile(ctx, id)
	if err != nil {
		return media.QualityPreset{CRF: 0, Preset: "", AudioKbps: 0, MaxWidth: 0}, false
	}

	return profile.QualityPreset(), true
}
