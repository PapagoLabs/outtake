// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/PapagoLabs/outtake/internal/app"
	"github.com/PapagoLabs/outtake/internal/cli/flags"
	"github.com/PapagoLabs/outtake/internal/config"
)

// NewStartCommand creates the server start command.
//
// Returns:
//   - command: The server start command.
func NewStartCommand() *cobra.Command {
	var serverFlags flags.ServerFlags

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the outtake server",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStart(&serverFlags)
		},
	}

	serverFlags.Bind(cmd.Flags())

	return cmd
}

// runStart executes the server start command.
//
// Parameters:
//   - serverFlags: Server flags.
//
// Returns:
//   - err: The error, if any.
func runStart(serverFlags *flags.ServerFlags) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if serverFlags.ListenAddr != "" {
		cfg.ListenAddr = serverFlags.ListenAddr
	}

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
