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
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// renderLogin renders the login page with the context it is handed.
//

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
	assert.Contains(t, body, "Login With Plex")
	assert.Contains(t, body, `data-status-url="/api/auth/status"`,
		"the sign-in script polls the status once the popup is open")
	assert.NotContains(t, body, `hx-trigger="every`,
		"a page nobody signs in from asks the server nothing")
	assert.Contains(t, body, "Waiting for Plex…")
	assert.Contains(t, body, `action="/api/auth/login"`,
		"a manual token can still be posted")
	assert.Contains(t, body, `name="token"`)
	assert.Contains(t, body, "Plex Token")
	assert.Contains(t, body, `src="`+assets.URL("js/login.js")+`"`)
}

// TestLoginShowsTheCarriedFailure covers a refused login: the failure the
// session carried to the page shows in its banner slot.
func TestLoginShowsTheCarriedFailure(t *testing.T) {
	t.Parallel()

	ctx := view.ContextWithFailure(
		t.Context(),
		view.NewNotice("Plex rejected that token. Check it and try again."),
	)
	body := renderLogin(t, ctx, LoginProps{})

	assert.Contains(t, body, `<div id="flash">`)
	assert.Contains(t, body, "Plex rejected that token. Check it and try again.")
}

func TestLoginOmitsTheErrorBannerWhenThereIsNoError(t *testing.T) {
	t.Parallel()

	body := renderLogin(t, t.Context(), LoginProps{AuthURL: "https://plex.tv/link"})

	assert.NotContains(t, body, "js-flash")
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
	assert.Contains(t, body, `placeholder="Paste a Plex token"`)
	assert.Contains(t, body, `<label for="manual-token"`)
}
