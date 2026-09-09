// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flags

import (
	"github.com/spf13/pflag"
)

// VersionFlags contains flags for the version command.
type VersionFlags struct {
	CommonFlags

	// JSON outputs version information in JSON format.
	JSON bool
}

// Bind attaches version-specific flags to the provided flag set.
//
// Parameters:
//   - flags: Cobra/pflag set receiving shared flags.
func (vf *VersionFlags) Bind(flags *pflag.FlagSet) {
	flags.BoolVar(
		&vf.JSON,
		"json",
		false,
		"Output version information in JSON format",
	)
}
