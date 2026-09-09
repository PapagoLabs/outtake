// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package health provides the health check CLI command.
package health

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PapagoLabs/outtake/internal/app/health"
	"github.com/PapagoLabs/outtake/internal/cli/flags"
	"github.com/PapagoLabs/outtake/internal/config"
)

// NewCommand creates the health command.
//
// Returns:
//   - command: The health command.
func NewCommand() *cobra.Command {
	var serverFlags flags.ServerFlags

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check the health of the outtake server",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runHealth(&serverFlags)
		},
	}

	serverFlags.Bind(cmd.Flags())

	return cmd
}

// runHealth executes the server health command.
//
// Parameters:
//   - serverFlags: Typed *flags.ServerFlags argument for runHealth.
//
// Returns:
//   - err: Wrapped failure such as "load config" or "health check".
func runHealth(serverFlags *flags.ServerFlags) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if serverFlags.ListenAddr != "" {
		cfg.ListenAddr = serverFlags.ListenAddr
	}

	checker := health.NewChecker()

	err = checker.Check(cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	return nil
}
