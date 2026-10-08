// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipprofile "github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// postedProfileFields reads a posted profile form the way the handler does.
//
// Parameters:
//   - t: The test the request belongs to.
//   - form: Encoded form body.
//
// Returns:
//   - fields: The raw values the form carried.
func postedProfileFields(t *testing.T, form string) clipprofile.ProfileFields {
	t.Helper()

	app := fiber.New()

	var fields clipprofile.ProfileFields

	app.Post(routes.PathSettingsProfiles, func(ctx fiber.Ctx) error {
		fields = profileFields(ctx)

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, routes.PathSettingsProfiles, strings.NewReader(form),
	)
	req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	return fields
}

func TestProfileFieldsDecodeTheForm(t *testing.T) {
	t.Parallel()

	fields := postedProfileFields(
		t,
		"name=Archive&crf=18&preset=slow&audioKbps=320&maxWidth=3840&isDefault=1&keepHdr=1",
	)

	assert.Equal(t, "Archive", fields.Name)
	assert.Equal(t, "18", fields.CRF)
	assert.Equal(t, "slow", fields.Preset)
	assert.Equal(t, "320", fields.AudioKbps)
	assert.Equal(t, "3840", fields.MaxWidth)
	assert.True(t, fields.IsDefault)
	assert.True(t, fields.KeepHDR)
}

func TestProfileFieldsReadAnUncheckedDefaultAsOff(t *testing.T) {
	t.Parallel()

	fields := postedProfileFields(
		t,
		"name=Archive&crf=18&preset=slow&audioKbps=320&maxWidth=3840",
	)

	assert.False(t, fields.IsDefault)
	assert.False(t, fields.KeepHDR, "an unchecked Keep HDR box converts to SDR")
}

// closeBody closes a response body and fails the test when it cannot.
//
// Parameters:
//   - t: The test the response belongs to.
//   - resp: The response to close.
func closeBody(t *testing.T, resp *http.Response) {
	t.Helper()

	require.NoError(t, resp.Body.Close())
}
