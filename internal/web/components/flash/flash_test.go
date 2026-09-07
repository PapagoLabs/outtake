// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flash

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBannerOmitsEmptyMessage(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Banner("").Render(t.Context(), &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestPartialTargetsFlash(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Partial("clip is not running").Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `<hx-partial hx-target="#flash">`)
	assert.Contains(t, body, "clip is not running")
	assert.Contains(t, body, "js-flash")
}

func TestSlotHasFlashID(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Slot("").Render(t.Context(), &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `id="flash"`)
}
