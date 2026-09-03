// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestMediaSearchPreservesLibrary(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Media(view.MediaProps{
		LibraryID: "7",
		Query:     "matrix",
	}).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `name="library"`)
	assert.Contains(t, body, `value="7"`)
	assert.Contains(t, body, `name="q"`)
}
