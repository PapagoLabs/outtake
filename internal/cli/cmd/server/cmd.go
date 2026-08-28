// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package server provides server-related CLI commands.
package server

import (
	"github.com/spf13/cobra"
)

// NewCommand creates the server command and its subcommands.
func NewCommand() *cobra.Command {
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Manage the outtake server",
	}

	serverCmd.AddCommand(NewStartCommand())

	return serverCmd
}
