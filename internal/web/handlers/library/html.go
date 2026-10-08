// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// PlexAuth is the slice of Plex authentication the HTML pages depend on.
type PlexAuth interface {
	// Client returns a Plex client for the selected server.
	Client() (*plex.Client, plex.Server, bool)

	// Discover lists the Plex servers an access token can reach.
	Discover(ctx context.Context, accessToken string) ([]plex.Server, error)

	// Select binds the application to a Plex server.
	Select(ctx context.Context, server plex.Server) error

	// Selected returns the bound Plex server.
	Selected() (plex.Server, bool)

	// Sessions returns the Plex sessions currently playing.
	Sessions() []plex.Session
}

// MediaDescriber describes the media a page renders.
type MediaDescriber interface {
	// Describe resolves a Plex media item and reports what the file behind it
	// holds.
	Describe(ctx context.Context, mediaID string) library.SourceInfo

	// DescribePath reports what a local media file holds.
	DescribePath(ctx context.Context, path string) library.SourceInfo
}

// OutputStore reports whether a rendered clip is stored.
type OutputStore interface {
	// Exists reports whether the object at path exists, without downloading it.
	Exists(ctx context.Context, path string) bool
}

// Handler handles HTML page requests.
type Handler struct {
	queue   *queue.Queue
	outputs OutputStore
	db      *database.DB
	auth    PlexAuth
	cfg     *config.Config
	sources MediaDescriber
	// addedAt caches the jump-rail buckets the media page collects, and starts
	// empty, so it needs no wiring.
	addedAt library.AddedAtCache
	// libraries caches each server's library list for the sidebar and every
	// browse fragment, and starts empty.
	libraries library.LibraryCache
}

// New creates a new HTML handler.
//
// Parameters:
//   - jobQueue: Clip queue the in-memory job view is read from.
//   - db: Persistence handle.
//   - auth: Plex authentication, which owns the selected server.
//   - cfg: Application configuration.
//   - sources: Describer for the media a page renders.
//   - outputs: Storage the media page checks clip files against.
//
// Returns:
//   - handler: A ready-to-use HTML page handler.
func New(
	jobQueue *queue.Queue,
	db *database.DB,
	auth PlexAuth,
	cfg *config.Config,
	sources MediaDescriber,
	outputs OutputStore,
) *Handler {
	return &Handler{
		queue:   jobQueue,
		outputs: outputs,
		db:      db,
		auth:    auth,
		cfg:     cfg,
		sources: sources,
	}
}

// plexPair returns a client for the currently selected server.
//
// Returns:
//   - client: PMS client scoped to the selected server.
//   - server: The selected server.
//   - ok: False when no server has been selected yet.
func (handler *Handler) plexPair() (*plex.Client, plex.Server, bool) {
	return handler.auth.Client()
}

// sidebarLibraries lists libraries for the sidebar.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - libraries: Page models for every library, or nil when Plex is unreachable.
func (handler *Handler) sidebarLibraries(ctx fiber.Ctx) []view.LibraryItem {
	plexClient, server, ok := handler.plexPair()
	if !ok {
		return nil
	}

	libs, err := handler.libraries.Libraries(ctx.Context(), plexClient, server)
	if err != nil {
		return nil
	}

	return view.LibraryItems(libs)
}
