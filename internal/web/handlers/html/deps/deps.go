// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package deps

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/plex"
)

// ClipJobs reads in-memory clip jobs for HTML pages.
type ClipJobs interface {
	GetJob(id string) *queue.Job
	GetAllJobs() []*queue.Job
}

// ClipStore loads and mutates persisted clips and profiles.
type ClipStore interface {
	GetClip(ctx context.Context, id string) (*queue.Job, error)
	ListClips(ctx context.Context) ([]*queue.Job, error)
	ListClipsForMedia(ctx context.Context, mediaID string) ([]*queue.Job, error)
	SaveClipProfile(ctx context.Context, profile database.ClipProfile) error
	GetClipProfile(ctx context.Context, id string) (database.ClipProfile, error)
	ListClipProfiles(ctx context.Context) ([]database.ClipProfile, error)
	SetDefaultClipProfile(ctx context.Context, id string) error
	DeleteClipProfile(ctx context.Context, id string) error
	SaveSelectedServer(ctx context.Context, server plex.Server) error
}

// ServerBinding exposes the selected Plex server and live sessions.
type ServerBinding interface {
	Get() (plex.Server, bool)
	Set(server plex.Server)
	Sessions() []plex.Session
}

// Deps bundles HTML handler dependencies behind interface boundaries.
type Deps struct {
	Queue    ClipJobs
	DB       ClipStore
	Bind     ServerBinding
	Cfg      *config.Config
	Product  string
	ClientID string
}

// ClipMaxDur returns the configured maximum clip duration in seconds.
func (d Deps) ClipMaxDur() int {
	if d.Cfg != nil && d.Cfg.MaxClipDurSec > 0 {
		return d.Cfg.MaxClipDurSec
	}

	return 600
}
