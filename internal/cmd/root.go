// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package cmd provides the outtake CLI commands.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PapagoLabs/outtake/internal/cmd/health"
	"github.com/PapagoLabs/outtake/internal/cmd/owner"
	"github.com/PapagoLabs/outtake/internal/cmd/server"
	"github.com/PapagoLabs/outtake/internal/cmd/version"
)

// Execute runs the CLI.
//
// Returns:
//   - error: Non-nil when a command fails.
func Execute() error {
	rootCmd := newRoot()

	silenceUsageOnRunErrors(rootCmd)

	err := rootCmd.Execute()
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	return nil
}

// DocRoot builds the command tree the binary runs, for the CLI reference.
// Building it reads no configuration and touches no file or network, because
// every command does its work only when it runs.
//
// Returns:
//   - root: The root command with every subcommand attached.
func DocRoot() *cobra.Command {
	return newRoot()
}

// newRoot builds the outtake command tree.
//
// Returns:
//   - root: The root command with every subcommand attached.
func newRoot() *cobra.Command {
	rootCmd := &cobra.Command{}

	rootCmd.Use = "outtake"
	rootCmd.Short = "Media clipper for Plex libraries"
	rootCmd.Long = "outtake creates video clips, GIFs, and screenshots from your Plex media server."

	rootCmd.AddCommand(server.NewCommand())
	rootCmd.AddCommand(health.NewCommand())
	rootCmd.AddCommand(owner.NewCommand())
	rootCmd.AddCommand(version.NewCommand())

	return rootCmd
}

// silenceUsageOnRunErrors keeps the usage text for a mistyped command, flag,
// or argument, which cobra reports before a command runs, and leaves it out of
// an error the command returns while running, such as a data directory it
// cannot write.
//
// Parameters:
//   - command: The command whose tree is wrapped.
func silenceUsageOnRunErrors(command *cobra.Command) {
	if run := command.RunE; run != nil {
		command.RunE = func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			return run(cmd, args)
		}
	}

	for _, child := range command.Commands() {
		silenceUsageOnRunErrors(child)
	}
}
