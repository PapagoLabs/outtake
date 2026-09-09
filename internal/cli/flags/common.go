// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flags

import (
	"errors"

	"github.com/spf13/pflag"
)

// CommonFlags contains flags common to multiple commands.
type CommonFlags struct {
	// LogLevel sets the logging verbosity level (debug, info, warn, error).
	LogLevel string
	// Config is the path to the configuration file.
	Config string
	// Quiet suppresses all output except errors.
	Quiet bool
	// Verbose enables verbose output.
	Verbose bool
}

const (
	// DefaultTimeout is the default API call timeout in seconds.
	DefaultTimeout = 30
)

// errQuietVerboseConflict is returned when both quiet and verbose flags are set.
var errQuietVerboseConflict = errors.New("quiet and verbose flags are mutually exclusive")

// Bind attaches the common flags to the provided flag set.
//
// Parameters:
//   - flags: The flag set to attach common flags to.
func (cf *CommonFlags) Bind(flags *pflag.FlagSet) {
	// --log-level, -l: Set the logging verbosity level
	flags.StringVarP(
		&cf.LogLevel,
		"log-level",
		"l",
		"info",
		"Set logging level (debug, info, warn, error)",
	)
	// --config, -c: Path to the configuration file
	flags.StringVarP(
		&cf.Config,
		"config",
		"c",
		"",
		"Path to config file",
	)
	// --quiet, -q: Suppress non-error output
	flags.BoolVarP(
		&cf.Quiet,
		"quiet",
		"q",
		false,
		"Suppress all output except errors",
	)
	// --verbose, -v: Enable detailed output
	flags.BoolVarP(
		&cf.Verbose,
		"verbose",
		"v",
		false,
		"Enable verbose output",
	)
}

// Validate checks the common flags for consistency.
//
// Returns:
//   - err: Non-nil when the call fails.
func (cf *CommonFlags) Validate() error {
	if cf.Quiet && cf.Verbose {
		return errQuietVerboseConflict
	}

	return nil
}
