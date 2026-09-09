// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package wire

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/plex"
)

// ClipPersister loads clip jobs when restoring the in-memory queue.
type ClipPersister interface {
	ListClips(ctx context.Context) ([]*queue.Job, error)
}

// ServerSource loads the last selected Plex server.
type ServerSource interface {
	SelectedServer(ctx context.Context) (plex.Server, bool, error)
}

// Binder applies a selected Plex server to the session binding.
type Binder interface {
	Set(server plex.Server)
}
