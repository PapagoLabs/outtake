// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package owner provides the CLI commands that manage which Plex account owns
// the installation.
package owner

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

const (
	// msgReset confirms that an owner was forgotten.
	msgReset = "Owner cleared, along with the server it bound. The next Plex account " +
		"to sign in claims outtake. Restart outtake if it is running.\n"

	// msgNoOwner reports that there was no owner to forget.
	msgNoOwner = "No owner was set. The next Plex account to sign in claims outtake.\n"
)

// NewCommand creates the owner command and its subcommands.
//
// Returns:
//   - ownerCmd: The owner command.
func NewCommand() *cobra.Command {
	ownerCmd := &cobra.Command{}

	ownerCmd.Use = "owner"
	ownerCmd.Short = "Manage the Plex account that owns outtake"

	ownerCmd.AddCommand(newResetCommand())

	return ownerCmd
}

// newResetCommand creates the owner reset command.
//
// Returns:
//   - cmd: The reset command.
func newResetCommand() *cobra.Command {
	cmd := &cobra.Command{}

	cmd.Use = "reset"
	cmd.Short = "Forget the owner so the next Plex account to sign in claims outtake"
	cmd.Long = "Forget the Plex account that owns outtake and the server it bound. " +
		"Every signed-in browser is signed out, and the next Plex account to sign in " +
		"becomes the owner."
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runReset(cmd.OutOrStdout())
	}

	return cmd
}

// runReset executes the owner reset command.
//
// Parameters:
//   - out: Where the result is reported.
//
// Returns:
//   - error: Non-nil when the config, the database, or the reset fails.
func runReset(out io.Writer) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := database.NewFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	defer func() {
		_ = db.Close()
	}()

	removed, err := db.ResetOwner(context.Background())
	if err != nil {
		return fmt.Errorf("reset owner: %w", err)
	}

	message := msgNoOwner
	if removed {
		message = msgReset
	}

	_, err = io.WriteString(out, message)
	if err != nil {
		return fmt.Errorf("report reset: %w", err)
	}

	return nil
}
