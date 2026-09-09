// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/theme"
)

func TestAppearanceListsPalettes(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Appearance().Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "Appearance")
	assert.Contains(t, body, `data-palette="plex"`)
	assert.Contains(t, body, `data-palettes="`+theme.IDList()+`"`)

	for _, palette := range theme.Palettes() {
		assert.Contains(t, body, `data-palette="`+palette.ID+`"`)
		assert.Contains(t, body, palette.Name)
	}
}
