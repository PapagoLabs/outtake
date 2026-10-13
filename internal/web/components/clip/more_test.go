// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMoreClipsLoadsTheNextPageAsItScrollsIn covers the list's sentinel.
//
// It loads the next page once it is revealed and is replaced by what comes
// back. A list with nothing more renders no sentinel.
func TestMoreClipsLoadsTheNextPageAsItScrollsIn(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	require.NoError(t, MoreClips("clip-list-more", "/clips?offset=24").Render(t.Context(), &buf))

	body := buf.String()
	assert.Contains(t, body, `id="clip-list-more"`)
	assert.Contains(t, body, `hx-get="/clips?offset=24"`)
	assert.Contains(t, body, `hx-trigger="revealed"`)
	assert.Contains(t, body, `hx-swap="outerHTML"`)

	buf.Reset()
	require.NoError(t, MoreClips("clip-list-more", "").Render(t.Context(), &buf))
	assert.Empty(t, buf.String())
}
