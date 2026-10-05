// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
)

func TestAuthCompleteCarriesTheNextPage(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := AuthComplete("/servers").Render(t.Context(), &buf)
	require.NoError(t, err)

	page := buf.String()

	assert.Contains(t, page, `data-auth-next="/servers"`)
	assert.Contains(t, page, "/assets/js/auth-complete.js",
		"the page hands the opener back through the shared script")
	assert.Contains(t, page, "You can close this window.")
}

func TestAuthCompleteCarriesTheCSRFToken(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	ctx := identity.ContextWithCSRFToken(t.Context(), "complete-csrf")
	err := AuthComplete("/clips").Render(ctx, &buf)
	require.NoError(t, err)

	page := buf.String()
	assert.Contains(t, page, `<meta name="csrf-token" content="complete-csrf">`)
}

func TestAuthCompleteOmitsTheCSRFTokenWithoutOne(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := AuthComplete("").Render(t.Context(), &buf)
	require.NoError(t, err)

	page := buf.String()
	assert.NotContains(t, page, `<meta name="csrf-token"`)
	assert.Contains(t, page, `data-auth-next=""`)
}
