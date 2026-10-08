// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// ServerSelection is the Plex server selection a media path is resolved against.
type ServerSelection interface {
	// Get returns the selected server.
	Get() (plex.Server, bool)

	// Client builds a Plex client for the selected server.
	Client() (*plex.Client, plex.Server, bool)
}

var (
	// ErrNoServer is returned when no Plex server is selected.
	ErrNoServer = errors.New("no plex server selected")

	// ErrDolbyVisionBaseLayer is returned for a Dolby Vision source, such as
	// profile 5, whose base layer only shows correct colors after Dolby's
	// reshaping, which ffmpeg does not apply.
	ErrDolbyVisionBaseLayer = errors.New(
		"this source is Dolby Vision profile 5, which cannot be exported with " +
			"correct colors: use a copy with an HDR10 base layer",
	)
)

// ResolveMediaPath maps a media id onto a local filesystem path.
//
// Parameters:
//   - ctx: Request context.
//   - cfg: Configuration carrying the environment and the media remap.
//   - selected: Plex server selection to resolve against.
//   - mediaID: Plex media item id.
//
// Returns:
//   - path: Remapped local path for the media file.
//   - err: ErrNoServer when no server is selected, or the Plex failure.
func ResolveMediaPath(
	ctx context.Context,
	cfg *config.Config,
	selected ServerSelection,
	mediaID string,
) (string, error) {
	// E2E tests pass a local file path as the media id.
	if local, ok := localMediaFile(cfg.Env, mediaID); ok {
		return local, nil
	}

	client, server, ok := selected.Client()
	if !ok {
		return "", ErrNoServer
	}

	resolved, err := client.GetMediaPath(ctx, server, mediaID)
	if err != nil {
		return "", fmt.Errorf("resolve media path: %w", err)
	}

	return cfg.RemapMediaPath(resolved), nil
}

// localMediaFile reports whether a media id names a readable local file.
//
// Parameters:
//   - env: Configured environment name.
//   - mediaID: Plex media item id, which is a path under e2e.
//
// Returns:
//   - path: The media id as a local path.
//   - ok: True when the id names an existing non-directory file.
func localMediaFile(env, mediaID string) (string, bool) {
	if env != config.EnvE2E {
		return "", false
	}

	info, err := os.Stat(mediaID)
	if err != nil || info.IsDir() {
		return "", false
	}

	return mediaID, true
}
