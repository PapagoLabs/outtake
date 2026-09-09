// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

import (
	"strings"
)

// CSS renders :root/.dark for the default palette and data-palette overrides.
//
// Returns:
//   - value: The value.
func CSS() string {
	var buf strings.Builder

	_, _ = buf.WriteString("/* Generated from internal/web/theme. Do not edit. */\n")

	palettes := Palettes()
	writeBlock(&buf, ":root", &palettes[0].Light)
	writeBlock(&buf, ".dark", &palettes[0].Dark)

	for i := 1; i < len(palettes); i++ {
		palette := &palettes[i]
		writeBlock(&buf, `html[data-palette="`+palette.ID+`"]`, &palette.Light)
		writeBlock(&buf, `html[data-palette="`+palette.ID+`"].dark`, &palette.Dark)
	}

	return buf.String()
}

// writeBlock writes one selector's CSS variables.
//
// Parameters:
//   - buf: Buf.
//   - selector: Selector.
//   - scheme: Scheme.
func writeBlock(buf *strings.Builder, selector string, scheme *Scheme) {
	tokens := scheme.tokens()

	_, _ = buf.WriteString("\n")
	_, _ = buf.WriteString(selector)
	_, _ = buf.WriteString(" {\n")

	for _, pair := range tokens.pairs() {
		_, _ = buf.WriteString("  --")
		_, _ = buf.WriteString(pair[0])
		_, _ = buf.WriteString(": ")
		_, _ = buf.WriteString(pair[1])
		_, _ = buf.WriteString(";\n")
	}

	_, _ = buf.WriteString("}\n")
}
