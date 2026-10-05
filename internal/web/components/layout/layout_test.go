// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package layout

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
)

func TestLayoutBoostsSidebarIntoMain(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Layout(Props{Title: "Dashboard", Active: "dashboard", Library: "7"}).
		Render(identity.ContextWithCSRFToken(t.Context(), "csrf-test"), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `hx-boost:inherited="true"`)
	assert.Contains(t, body, `hx-target:inherited="#main-content"`)
	assert.NotContains(t, body, `hx-select:inherited`)
	assert.Contains(t, body, `hx-swap:inherited="outerHTML scroll:top scrollTarget:#main-content"`)
	assert.Contains(t, body, `class="dark h-dvh overflow-hidden"`)
	assert.Contains(t, body, "h-dvh")
	assert.Contains(t, body, "overflow-hidden")
	assert.Contains(t, body, "overscroll-contain")
	assert.NotContains(t, body, "min-h-screen")
	assert.NotContains(t, body, "scroll:window:top")
	assert.Contains(t, body, `hx-headers:inherited`)
	assert.Contains(t, body, "X-Csrf-Token")
	assert.Contains(t, body, `id="main-content"`)
	assert.Contains(t, body, `hx-history-elt`)
	assert.Contains(t, body, `id="flash"`)
	assert.Contains(t, body, `data-nav="dashboard"`)
	assert.Contains(t, body, `data-library="7"`)
	assert.Contains(t, body, `data-nav="media"`)
	assert.Contains(t, body, `data-nav="clips"`)
	assert.Regexp(t, `id="nav-libraries"[^>]*hx-target="this"`, body)
	assert.NotRegexp(t, `id="nav-libraries"[^>]*hx-select`, body)
	assertLogoutForm(t, body)

	for _, marker := range []string{
		`flex items-center gap-2.5`,
		`data-nav="dashboard"`,
		`data-nav="media"`,
		`data-nav="clips"`,
		`data-nav="servers"`,
		`data-nav="profiles"`,
		`data-nav="appearance"`,
	} {
		assertAnchorSelectsMain(t, body, marker)
	}
}

func assertAnchorSelectsMain(t *testing.T, body, marker string) {
	t.Helper()

	for _, tag := range regexp.MustCompile(`<a\b[^>]*>`).FindAllString(body, -1) {
		if strings.Contains(tag, marker) {
			assert.Contains(t, tag, `hx-select="#main-content"`)

			return
		}
	}

	require.Failf(t, "missing nav link", "no anchor contains %q", marker)
}

func assertLogoutForm(t *testing.T, body string) {
	t.Helper()

	tag := regexp.MustCompile(`<form[^>]*action="/api/auth/logout"[^>]*>`).FindString(body)
	require.NotEmpty(t, tag, "missing logout form")

	assert.Contains(t, tag, `method="POST"`)
	assert.Contains(t, tag, `hx-boost="false"`)
	assert.Contains(t, body, `<input type="hidden" name="_csrf" value="csrf-test">`)
	assert.Regexp(t, `<button[^>]*type="submit"[^>]*>Logout</button>`, body)
	assert.NotContains(t, body, `href="/api/auth/logout"`)
}
