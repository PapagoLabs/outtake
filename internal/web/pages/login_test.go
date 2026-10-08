// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/assets"
)

// renderLogin renders the login page with the context it is handed.
//
//nolint:contextcheck // templ renders the page with the context it is given.
func renderLogin(t *testing.T, ctx context.Context, props LoginProps) string {
	t.Helper()

	var buf strings.Builder

	err := Login(props).Render(ctx, &buf)
	require.NoError(t, err)

	return buf.String()
}

func TestLoginOffersBothWaysIn(t *testing.T) {
	t.Parallel()

	body := renderLogin(t, t.Context(), LoginProps{})

	assert.Contains(t, body, `id="plex-login"`, "the Plex popup sign-in is offered")
	assert.Contains(t, body, "Sign in with Plex")
	assert.Contains(t, body, `hx-get="/api/auth/status"`, "the page polls the sign-in status")
	assert.Contains(t, body, "Waiting for Plex authorization...")
	assert.Contains(t, body, `action="/api/auth/login"`,
		"a manual token can still be posted")
	assert.Contains(t, body, `name="token"`)
	assert.Contains(t, body, "Plex Token")
	assert.Contains(t, body, `src="`+assets.URL("js/login.js")+`"`)
}

func TestLoginShowsTheSignInError(t *testing.T) {
	t.Parallel()

	body := renderLogin(t, t.Context(), LoginProps{Error: "Plex rejected that token."})

	assert.Contains(
		t,
		body,
		`<div class="mb-4 rounded-md bg-destructive/10 border `+`border-destructive/20 p-3 text-sm text-destructive">`,
	)
	assert.Contains(t, body, ">Plex rejected that token.</div>")
}

func TestLoginOmitsTheErrorBannerWhenThereIsNoError(t *testing.T) {
	t.Parallel()

	body := renderLogin(t, t.Context(), LoginProps{AuthURL: "https://plex.tv/link"})

	assert.NotContains(t, body, ">Plex rejected that token.<")
	assert.Contains(t, body, `<div id="plex-error" class="mb-4 hidden`,
		"the placeholder the sign-in script writes into is always rendered")
}

func TestLoginCarriesTheCSRFToken(t *testing.T) {
	t.Parallel()

	ctx := identity.ContextWithCSRFToken(t.Context(), "login-csrf")
	body := renderLogin(t, ctx, LoginProps{})

	assert.Contains(t, body, `<meta name="csrf-token" content="login-csrf">`)
	assert.Contains(t, body, "hx-headers:inherited")
	assert.Contains(t, body, "X-Csrf-Token")
	assert.Contains(t, body, `<input type="hidden" name="_csrf" value="login-csrf">`)
}

func TestLoginOmitsTheCSRFMetaWithoutAToken(t *testing.T) {
	t.Parallel()

	body := renderLogin(t, t.Context(), LoginProps{})

	assert.NotContains(t, body, `<meta name="csrf-token"`)
	assert.NotContains(t, body, "hx-headers:inherited")
	assert.NotContains(t, body, "X-Csrf-Token")
	assert.NotContains(t, body, `name="_csrf"`)
}

func TestLoginKeepsTheManualTokenFieldHidden(t *testing.T) {
	t.Parallel()

	body := renderLogin(t, t.Context(), LoginProps{})

	assert.Contains(t, body, `id="manual-token"`)
	assert.Contains(t, body, `type="password"`)
	assert.Contains(t, body, `placeholder="Enter token manually"`)
	assert.Contains(t, body, `<label for="manual-token"`)
}
