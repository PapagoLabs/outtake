// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package brand

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIconHeadLinksEveryIconSize(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := IconHead().Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()

	for _, want := range []string{
		`<link rel="icon" href="/assets/brand/favicon.ico" sizes="48x48">`,
		`<link rel="icon" type="image/png" sizes="32x32" href="/assets/brand/favicon-32x32.png">`,
		`<link rel="icon" type="image/png" sizes="16x16" href="/assets/brand/favicon-16x16.png">`,
		`<link rel="apple-touch-icon" href="/assets/brand/apple-touch-icon.png">`,
	} {
		assert.Contains(t, body, want)
	}
}
