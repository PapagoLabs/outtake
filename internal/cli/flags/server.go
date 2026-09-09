// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flags

import (
	"github.com/spf13/pflag"
)

// ServerFlags contains flags specific to the server start command.
type ServerFlags struct {
	// ListenAddr is the address the server listens on.
	ListenAddr string
}

// Bind attaches server-specific flags to the provided flag set.
//
// Parameters:
//   - flags: Cobra/pflag set receiving shared flags.
func (sf *ServerFlags) Bind(flags *pflag.FlagSet) {
	flags.StringVar(
		&sf.ListenAddr,
		"listen",
		"",
		"Listen address (overrides config file)",
	)
}
