// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/playback"
	"github.com/PapagoLabs/outtake/internal/web/respond/respondtest"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// previewSettingsApp mounts the preview settings routes.
//
// Parameters:
//   - t: The test the app belongs to.
//   - handler: The handler under test.
//
// Returns:
//   - app: An app serving the page and its form.
func previewSettingsApp(t *testing.T, handler *Handler) *fiber.App {
	t.Helper()

	app := fiber.New()
	respondtest.Sessions(t, app)
	app.Get(routes.PathSettingsPreviews, handler.PreviewSettings)
	app.Post(routes.PathSettingsPreviews, handler.SavePreviewSettings)

	return app
}

// TestPreviewSettingsOfferTheMaximumResolutions covers the page: it offers
// 720p, 1080p, and 4K and marks the one in use, 1080p until another is chosen.
func TestPreviewSettingsOfferTheMaximumResolutions(t *testing.T) {
	t.Parallel()

	handler, _ := pageHandler(t, offlineAuth(t), silentSources(t))

	answer := serve(t, previewSettingsApp(t, handler), routes.PathSettingsPreviews, false)

	require.Equal(t, fiber.StatusOK, answer.status)
	assertBodyContains(t, answer.body, `<option value="1280">720p</option>`, "720p is offered")
	assertBodyContains(t, answer.body, `<option value="3840">4K</option>`, "4K is offered")
	assertBodyContains(t, answer.body, "Maximum Preview Resolution", "the setting is named")
	assertBodyContains(t, answer.body, `<option value="1920" selected>1080p</option>`,
		"1080p is in use until another is chosen")
	assertBodyContains(t, answer.body, `name="maxPreviewWidth"`, "the form posts the resolution")
}

// TestSavePreviewSettingsStoresTheChosenResolution covers a save: an offered
// resolution is stored and the page shows it, and any other is refused with
// the reason while the stored one stays.
func TestSavePreviewSettingsStoresTheChosenResolution(t *testing.T) {
	t.Parallel()

	handler, db := pageHandler(t, offlineAuth(t), silentSources(t))
	app := previewSettingsApp(t, handler)

	saved := serveMethod(t, app, http.MethodPost, routes.PathSettingsPreviews, false, "",
		url.Values{"maxPreviewWidth": {"1280"}}.Encode())

	require.Equal(t, fiber.StatusSeeOther, saved.status)
	assert.Equal(t, routes.PathSettingsPreviews, saved.header.Get(fiber.HeaderLocation))
	assert.Equal(t, clip.OutputWidth720p, playback.MaxPreviewWidth(t.Context(), db))

	refused := serveMethod(t, app, http.MethodPost, routes.PathSettingsPreviews, false, "",
		url.Values{"maxPreviewWidth": {"2560"}}.Encode())

	require.Equal(t, fiber.StatusSeeOther, refused.status)
	assert.Equal(t, routes.PathSettingsPreviews, refused.header.Get(fiber.HeaderLocation))
	assert.Equal(t, "Choose 720p, 1080p, or 4K",
		respondtest.FlashForCookies(t, app, refused.cookies).Message, "the page is told why")
	assert.Equal(t, clip.OutputWidth720p, playback.MaxPreviewWidth(t.Context(), db),
		"a refused resolution changes nothing")
}
