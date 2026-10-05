// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"io/fs"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"
)

func TestAssetsEmbedsTheStaticTree(t *testing.T) {
	t.Parallel()

	entries, err := fs.ReadDir(Assets, "assets")
	require.NoError(t, err)
	require.NotEmpty(t, entries, "the embed directive matched a directory")

	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	assert.Subset(t, names, []string{"brand", "css", "js"},
		"the asset root carries every directory the pages link to")
}

func TestAssetsEmbedsFilesInEveryAssetDirectory(t *testing.T) {
	t.Parallel()

	for _, dir := range []string{"assets/brand", "assets/css", "assets/js"} {
		entries, err := fs.ReadDir(Assets, dir)
		require.NoError(t, err, dir)
		assert.NotEmpty(t, entries, dir)
	}
}

func TestStaticConfigServesTheEmbeddedAssets(t *testing.T) {
	t.Parallel()

	cfg := staticConfig()

	assert.Equal(t, Assets, cfg.FS)

	for _, asset := range []string{
		"assets/brand/favicon.ico",
		"assets/brand/mascot-128.png",
		"assets/js/htmx.min.js",
		"assets/css/palettes.css",
		"assets/css/input.css",
		"assets/css/output.css",
		"assets/js/theme.js",
	} {
		info, err := fs.Stat(cfg.FS, asset)

		require.NoError(t, err, asset)
		assert.Positive(t, info.Size(), asset)
	}
}

func TestStaticConfigHasNoEntryAtAnUnbuiltAssetPath(t *testing.T) {
	t.Parallel()

	_, err := fs.Stat(staticConfig().FS, "assets/css/not-built.css")

	require.Error(t, err, "a path nothing was embedded at must not resolve")
}

func TestStaticConfigServesTheCompiledStylesheetThroughTheRouter(t *testing.T) {
	t.Parallel()

	got := newBrowser(t).request(t, http.MethodGet, "/assets/css/output.css")

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.Contains(t, got.contentType, "text/css")
	assert.Contains(t, got.body, "tailwindcss")
}

func TestAssetsEmbedsTheStylesheetTheTailwindTaskBuilds(t *testing.T) {
	t.Parallel()

	body := readAsset(t, "assets/css/output.css")

	assert.Contains(t, body, "@layer",
		"the compiled stylesheet is the Tailwind output, not the entry file")
	assert.NotContains(t, body, "@tailwind",
		"the entry directives are resolved away in the compiled output")
}

// readAsset returns the contents of one embedded asset.
//
// Parameters:
//   - t: The test that read the asset.
//   - asset: Path of the asset inside the embedded file system.
//
// Returns:
//   - body: The asset contents as a string.
func readAsset(t *testing.T, asset string) string {
	t.Helper()

	body, err := fs.ReadFile(Assets, asset)
	require.NoError(t, err)

	return string(body)
}
