// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package version provides the CLI command for displaying application version information.
package version

import (
	"fmt"
	"io"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	clilib "github.com/PapagoLabs/outtake/internal/cli"
	"github.com/PapagoLabs/outtake/internal/cli/flags"
	"github.com/PapagoLabs/outtake/internal/cli/metadata"
)

// NewCommand creates the version command.
//
// Returns:
//   - *cobra.Command: The version command that prints application version information.
func NewCommand() *cobra.Command {
	vflags := &flags.VersionFlags{ //nolint:modernize // embedlit wants a construct Go 1.27 still rejects.
		CommonFlags: flags.CommonFlags{
			LogLevel: "",
			Config:   "",
			Quiet:    false,
			Verbose:  false,
		},
		JSON: false,
	}

	cmd := clilib.Command()

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
func runVersionCmd(cmd *cobra.Command, vflags *flags.VersionFlags) error {
	printer, err := selectPrinter(cmd, vflags)
	if err != nil {
		return fmt.Errorf("select printer: %w", err)
	}

	log.Debug().
		Str("format", map[bool]string{true: "json", false: "text"}[vflags.JSON]).
		Msg("printing version")

	err = printer(cmd.OutOrStdout())
	if err != nil {
		return fmt.Errorf("print version: %w", err)
	}

	return nil
}

// selectPrinter returns the appropriate version output printer based on flags.
//
// Parameters:
//   - cmd: Cobra command providing access to the verbose flag.
//   - vflags: Version flags; if JSON is true, returns the JSON printer.
//
// Returns:
//   - func(io.Writer) error: The selected printer function.
//   - error: Non-nil if the verbose flag cannot be retrieved.
func selectPrinter(cmd *cobra.Command, vflags *flags.VersionFlags) (func(io.Writer) error, error) {
	if vflags.JSON {
		return metadata.PrintJSON, nil
	}

	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return nil, fmt.Errorf("get verbose flag: %w", err)
	}

	if verbose {
		return metadata.PrintVerbose, nil
	}

	return metadata.PrintDefault, nil
}
