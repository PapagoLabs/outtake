// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"

	"github.com/PapagoLabs/outtake/internal/cli/cmd"
	"github.com/PapagoLabs/outtake/internal/logging"
)

// main is the entry point for the application.
func main() {
	logging.Init()

	err := cmd.Execute()
	if err != nil {
		logging.Logger.Error().Err(err).Msg("execution failed")
		os.Exit(1)
	}
}
