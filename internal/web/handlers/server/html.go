// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// PlexAuth is the slice of Plex authentication the HTML pages depend on.
type PlexAuth interface {
	// Client returns a Plex client for the selected server.
	Client() (*plex.Client, plex.Server, bool)

	// Discover lists the Plex servers an access token can reach.
	Discover(ctx context.Context, accessToken string) ([]plex.Server, error)

	// ForgetServer drops the bound Plex server and its persisted record.
	ForgetServer(ctx context.Context) error

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
}

// Handler handles HTML page requests.
type Handler struct {
	queue   *queue.Queue
	db      *database.DB
	auth    PlexAuth
	cfg     *config.Config
	sources MediaDescriber
}

// New creates a new HTML handler.
//
// Parameters:
//   - jobQueue: Clip queue the in-memory job view is read from.
//   - db: Persistence handle.
//   - auth: Plex authentication, which owns the selected server.
//   - cfg: Application configuration.
//   - sources: Describer for the media a page renders.
//
// Returns:
//   - handler: A ready-to-use HTML page handler.
func New(
	jobQueue *queue.Queue,
	db *database.DB,
	auth PlexAuth,
	cfg *config.Config,
	sources MediaDescriber,
) *Handler {
	return &Handler{
		queue:   jobQueue,
		db:      db,
		auth:    auth,
		cfg:     cfg,
		sources: sources,
	}
}
