// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package version provides the CLI command for displaying application version information.
package version

import (
	"fmt"
	"io"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/PapagoLabs/outtake/internal/metadata"
	"github.com/PapagoLabs/outtake/internal/settings/flags"
)

// NewCommand creates the version command.
//
// Returns:
//   - *cobra.Command: The version command that prints application version information.
func NewCommand() *cobra.Command {
	vflags := &flags.Version{}

	cmd := &cobra.Command{}

	cmd.Use = "version"
	cmd.Short = "Print the application version"
	cmd.Long = "Print the application version, including commit SHA and build details."
	cmd.Example = `  # Print version
  outtake version

  # Print detailed version info
  outtake version --verbose

  # Print version as JSON
  outtake version --json`
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runVersionCmd(cmd, vflags)
	}

	vflags.Bind(cmd.Flags())

	return cmd
}

// runVersionCmd executes the version command.
//
// Parameters:
//   - cmd: Cobra command used for output.
//   - vflags: Version flags controlling JSON output and verbose mode.
//
// Returns:
//   - error: Non-nil if the printer cannot be selected or output fails.
func runVersionCmd(cmd *cobra.Command, vflags *flags.Version) error {
	log.Debug().
		Str("format", map[bool]string{true: "json", false: "text"}[vflags.JSON]).
		Msg("printing version")

	err := selectPrinter(vflags)(cmd.OutOrStdout())
	if err != nil {
		return fmt.Errorf("print version: %w", err)
	}

	return nil
}

// selectPrinter returns the version output printer the flags select.
//
// Parameters:
//   - vflags: Version flags, which select the printer.
//
// Returns:
//   - printer: The function that writes the version output.
func selectPrinter(vflags *flags.Version) func(io.Writer) error {
	switch {
	case vflags.JSON:
		return metadata.PrintJSON
	case vflags.Verbose:
		return metadata.PrintVerbose
	default:
		return metadata.PrintDefault
	}
}
