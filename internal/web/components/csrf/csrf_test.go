// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package csrf

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web"
)

func TestFieldRendersHiddenInput(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Field().Render(web.ContextWithCSRFToken(t.Context(), "tok"), &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `name="_csrf"`)
	assert.Contains(t, buf.String(), `value="tok"`)
}

func TestFieldOmitsWhenEmpty(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Field().Render(t.Context(), &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
