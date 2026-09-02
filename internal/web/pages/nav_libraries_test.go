// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNavLibrariesHighlightsSelected(t *testing.T) {
	t.Parallel()

	libs := []LibraryItem{
		{ID: "1", Title: "Movies", Type: "movie"},
		{ID: "2", Title: "TV Shows", Type: "show"},
	}

	var buf strings.Builder

	err := NavLibraries(libs, "2").Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Regexp(t, `href="/media\?library=2"[^>]*bg-sidebar-primary`, body)
	assert.NotRegexp(t, `href="/media\?library=1"[^>]*bg-sidebar-primary`, body)
}
