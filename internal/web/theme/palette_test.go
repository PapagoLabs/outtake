// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package theme

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPalettesIsRegistry(t *testing.T) {
	t.Parallel()

	palettes := Palettes()
	require.Len(t, palettes, 6)
	assert.Equal(t, Plex, palettes[0])
	assert.Equal(t, Neutral, palettes[1])
	assert.Equal(t, Nord, palettes[2])
	assert.Equal(t, Catppuccin, palettes[3])
	assert.Equal(t, Dracula, palettes[4])
	assert.Equal(t, TokyoNight, palettes[5])
	assert.Equal(t, PalettePlex, palettes[0].ID)
}

func TestPalettesHaveUniqueIDsAndSwatches(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, len(Palettes()))
	for _, palette := range Palettes() {
		_, exists := seen[palette.ID]
		assert.False(t, exists, palette.ID)

		seen[palette.ID] = struct{}{}
		assert.NotEmpty(t, palette.Name)
		assert.NotEmpty(t, palette.Accent)
		assert.NotEmpty(t, palette.Surface)
		assert.True(t, palette.Light.complete(), palette.ID+" light")
		assert.True(t, palette.Dark.complete(), palette.ID+" dark")
	}
}

func TestPalettesCSSMatchesCatalog(t *testing.T) {
	t.Parallel()

	want, err := os.ReadFile("../assets/css/palettes.css")
	require.NoError(t, err)
	assert.Equal(t, string(want), CSS())
}

func TestThemeJSReadsDataPalettes(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"../assets/js/theme.js", "../assets/js/app.js"} {
		js, err := os.ReadFile(path)
		require.NoError(t, err)

		body := string(js)
		assert.Contains(t, body, "data-palettes")
		assert.NotContains(t, body, "catppuccin")
	}
}
