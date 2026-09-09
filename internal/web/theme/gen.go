//go:build ignore

// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"os"

	"github.com/PapagoLabs/outtake/internal/web/theme"
)

// main generates theme CSS assets.
func main() {
	err := os.WriteFile("internal/web/assets/css/palettes.css", []byte(theme.CSS()), 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write palettes.css: %v\n", err)
		os.Exit(1)
	}
}
