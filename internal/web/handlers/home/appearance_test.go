// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/theme"
)

// "/settings/appearance" is the route the appearance page is served on.
// getAppearance serves one appearance page request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - target: Request target, including any query string.
//
// Returns:
//   - answer: The status, headers, and body the response carried.
func getAppearance(t *testing.T, handler *Handler, target string) pageAnswer {
	t.Helper()

	app := fiber.New()
	app.Get("/settings/appearance", handler.Appearance)

	return serve(t, app, target, false)
}

func TestAppearanceRendersEveryPalette(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := getAppearance(t, handler, "/settings/appearance")

	require.Equal(t, fiber.StatusOK, answer.status)

	palettes := make([]string, 0, len(theme.Palettes()))
	for _, palette := range theme.Palettes() {
		palettes = append(palettes, palette.ID)
	}

	assertBodyContains(t, answer.body,
		`data-palettes="`+strings.Join(palettes, ",")+`"`,
		"every registered palette has to be offered or the user cannot pick it")
}

func TestAppearanceOpensOnTheDefaultPalette(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := getAppearance(t, handler, "/settings/appearance")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `data-palette="`+theme.PalettePlex+`"`,
		"the page is themed with the default palette before anything is chosen")
}

func TestAppearanceOffersTheThemeToggle(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := getAppearance(t, handler, "/settings/appearance")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, "js-theme-toggle",
		"a page that offers palettes has to offer the light and dark switch too")
}

func TestAppearanceIgnoresACarriedPalette(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := getAppearance(t, handler, "/settings/appearance"+"?palette=nord")

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `data-palette="`+theme.PalettePlex+`"`,
		"the chosen palette lives in the browser, so a query cannot override it")
}
