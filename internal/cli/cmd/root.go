// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package cmd provides the outtake CLI commands.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PapagoLabs/outtake/internal/cli/cmd/health"
	"github.com/PapagoLabs/outtake/internal/cli/cmd/server"
	"github.com/PapagoLabs/outtake/internal/cli/cmd/version"
)

// Execute runs the CLI.
//
// Returns:
//   - err: The error, if any.
func Execute() error {
	rootCmd := &cobra.Command{
		Use:   "outtake",
		Short: "Media clipper for Plex libraries",
		Long:  "outtake creates video clips, GIFs, and screenshots from your Plex media server.",
	}

	rootCmd.AddCommand(server.NewCommand())
	rootCmd.AddCommand(health.NewCommand())
	rootCmd.AddCommand(version.NewCommand())

	err := rootCmd.Execute()
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	return nil
}
