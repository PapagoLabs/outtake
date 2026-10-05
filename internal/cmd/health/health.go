// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package health provides the health check CLI command.
package health

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PapagoLabs/outtake/internal/app/health"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/settings/flags"
)

// NewCommand creates the health command.
//
// Returns:
//   - cmd: The health command.
func NewCommand() *cobra.Command {
	listen := &flags.Listen{}

	cmd := &cobra.Command{}

	cmd.Use = "health"
	cmd.Short = "Check the health of the outtake server"
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		return runHealth(listen)
	}

	listen.Bind(cmd.Flags())

	return cmd
}

// runHealth executes the server health command.
//
// Parameters:
//   - listen: Listen override that may replace the configured address.
//
// Returns:
//   - error: Non-nil when the config fails to load or the check fails.
func runHealth(listen *flags.Listen) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	listen.Apply(cfg)

	checker := health.NewChecker()

	err = checker.Check(cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	return nil
}
