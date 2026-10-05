// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flags

import (
	"github.com/spf13/pflag"

	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// Listen is the address override the server commands share, applied over the
// configured listen address.
type Listen struct {
	// Addr is the address the flag set carries, empty when it was not passed.
	Addr string
}

// FlagListen is the name of the listen address override.
const FlagListen = "listen"

// Apply overrides the configured listen address when the flag was passed.
//
// Parameters:
//   - cfg: Configuration to override.
func (listen *Listen) Apply(cfg *config.Config) {
	if listen.Addr != "" {
		cfg.ListenAddr = listen.Addr
	}
}

// Bind attaches the listen override to a flag set.
//
// Parameters:
//   - flagSet: The flag set to attach the override to.
func (listen *Listen) Bind(flagSet *pflag.FlagSet) {
	flagSet.StringVar(&listen.Addr, FlagListen, "", "Listen address (overrides config file)")
}
