// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package layout

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web"
)

func TestLayoutBoostsSidebarIntoMain(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Layout(Props{Title: "Dashboard", Active: "dashboard", Library: "7"}).
		Render(web.ContextWithCSRFToken(t.Context(), "csrf-test"), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `hx-boost:inherited="true"`)
	assert.Contains(t, body, `hx-target:inherited="#main-content"`)
	assert.Contains(t, body, `hx-select:inherited="#main-content"`)
	assert.Contains(t, body, `hx-swap:inherited="outerHTML scroll:window:top"`)
	assert.Contains(t, body, `hx-headers:inherited`)
	assert.Contains(t, body, "X-Csrf-Token")
	assert.Contains(t, body, `id="main-content"`)
	assert.Contains(t, body, `hx-history-elt`)
	assert.Contains(t, body, `id="flash"`)
	assert.Contains(t, body, `data-nav="dashboard"`)
	assert.Contains(t, body, `data-library="7"`)
	assert.Contains(t, body, `data-nav="media"`)
	assert.Contains(t, body, `data-nav="clips"`)
	assert.Regexp(t, `href="/api/auth/logout"[^>]*hx-boost="false"`, body)
	assert.Regexp(t, `id="nav-libraries"[^>]*hx-target="this"`, body)
	assert.Regexp(t, `id="nav-libraries"[^>]*hx-select="unset"`, body)
}
