// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flags

import (
	"github.com/spf13/pflag"
)

// Version are the flags for the version command.
type Version struct {
	// JSON reports whether the version is printed as JSON.
	JSON bool
	// Verbose reports whether the version is printed in detail.
	Verbose bool
}

// The names of the version command flags.
const (
	// FlagJSON outputs version information in JSON format.
	FlagJSON = "json"
	// FlagVerbose outputs multi-line detailed version information.
	FlagVerbose = "verbose"
)

// Bind attaches the version flags to a flag set.
//
// Parameters:
//   - flagSet: The flag set to attach the version flags to.
func (version *Version) Bind(flagSet *pflag.FlagSet) {
	flagSet.BoolVar(&version.JSON, FlagJSON, false, "Output version information in JSON format")
	flagSet.BoolVar(
		&version.Verbose,
		FlagVerbose,
		false,
		"Output detailed version information",
	)
}
