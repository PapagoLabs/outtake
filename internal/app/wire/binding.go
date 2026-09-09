// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package wire

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/config"
	plexserver "github.com/PapagoLabs/outtake/internal/plex/server"
)

// RestoreBinding loads the selected Plex server from config or the database.
//
// Parameters:
//   - cfg: Application configuration.
//   - servers: Servers.
//   - bind: Bind.
func RestoreBinding(cfg *config.Config, servers ServerSource, bind Binder) {
	if server, ok := plexserver.ServerFromURL(cfg.PlexServerURL, cfg.PlexToken); ok {
		bind.Set(server)

		return
	}

	server, ok, err := servers.SelectedServer(context.Background())
	if err != nil {
		log.Warn().Err(err).Msg("failed to load selected server")

		return
	}

	if ok {
		bind.Set(server)
	}
}
