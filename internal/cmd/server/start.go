// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/PapagoLabs/outtake/internal/app"
	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/settings/flags"
)

// NewStartCommand creates the server start command.
//
// Returns:
//   - cmd: The start command.
func NewStartCommand() *cobra.Command {
	listen := &flags.Listen{}

	cmd := &cobra.Command{}

	cmd.Use = "start"
	cmd.Short = "Start the outtake server"
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		return runStart(listen)
	}

	listen.Bind(cmd.Flags())

	return cmd
}

// runStart executes the server start command.
//
// Parameters:
//   - listen: Listen override that may replace the configured address.
//
// Returns:
//   - error: Non-nil when the config fails to load, the app fails to build, or
//     the server stops with an error.
func runStart(listen *flags.Listen) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	listen.Apply(cfg)

	logging.InitFromConfig(cfg)

	log.Info().
		Str("listen_addr", cfg.ListenAddr).
		Msg("starting outtake")

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer application.Close()

	err = application.Run()
	if err != nil {
		return fmt.Errorf("run app: %w", err)
	}

	return nil
}
