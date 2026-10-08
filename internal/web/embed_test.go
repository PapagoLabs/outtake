// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/web/assets"
)

func TestAssetsEmbedsTheStaticTree(t *testing.T) {
	t.Parallel()

	entries, err := fs.ReadDir(assets.FS, ".")
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

	for _, dir := range []string{"brand", "css", "js"} {
		entries, err := fs.ReadDir(assets.FS, dir)
		require.NoError(t, err, dir)
		assert.NotEmpty(t, entries, dir)
	}
}

func TestStaticConfigServesTheEmbeddedAssets(t *testing.T) {
	t.Parallel()

	cfg := staticConfig()

	assert.Equal(t, assets.FS, cfg.FS)

	for _, asset := range []string{
		"brand/favicon.ico",
		"brand/mascot-128.png",
		"js/htmx.min.js",
		"css/palettes.css",
		"css/input.css",
		"css/output.css",
		"js/theme.js",
	} {
		info, err := fs.Stat(cfg.FS, asset)

		require.NoError(t, err, asset)
		assert.Positive(t, info.Size(), asset)
	}
}

func TestStaticConfigHasNoEntryAtAnUnbuiltAssetPath(t *testing.T) {
	t.Parallel()

	_, err := fs.Stat(staticConfig().FS, "css/not-built.css")

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

	body := readAsset(t, "css/output.css")

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

	body, err := fs.ReadFile(assets.FS, asset)
	require.NoError(t, err)

	return string(body)
}

// assetResponse fetches one asset through the router with the headers a
// browser sends.
//
// Parameters:
//   - t: The test that fetches the asset.
//   - target: Request path and query.
//
// Returns:
//   - status: The response status.
//   - header: The response headers.
func assetResponse(t *testing.T, target string) (int, http.Header) {
	t.Helper()

	app := New(testRouterDeps(t, testRouterDatabase(t)))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)

	req.Host = routerHost
	req.Header.Set(fiber.HeaderAcceptEncoding, "gzip")

	resp, err := app.Test(req, browserTestConfig())
	require.NoError(t, err)

	defer resp.Body.Close()

	return resp.StatusCode, resp.Header
}

// TestAnAssetNamedByItsContentsIsCachedForGood covers the hashed URL every
// page renders: it is cached for a year, and a stale or missing version makes
// the browser check back instead.
func TestAnAssetNamedByItsContentsIsCachedForGood(t *testing.T) {
	t.Parallel()

	status, header := assetResponse(t, assets.URL("css/output.css"))
	require.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, cacheForever, header.Get(fiber.HeaderCacheControl))

	for _, target := range []string{
		"/assets/css/output.css",
		"/assets/css/output.css?v=0000000000000000",
	} {
		status, header := assetResponse(t, target)
		require.Equal(t, fiber.StatusOK, status, target)
		assert.Equal(t, cacheRevalidate, header.Get(fiber.HeaderCacheControl), target)
	}
}

// TestTextAssetsAreCompressed covers compression of the stylesheet and scripts.
func TestTextAssetsAreCompressed(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"css/output.css", "js/htmx.min.js"} {
		status, header := assetResponse(t, assets.URL(name))

		require.Equal(t, fiber.StatusOK, status, name)
		assert.Equal(t, "gzip", header.Get(fiber.HeaderContentEncoding), name)
	}
}
