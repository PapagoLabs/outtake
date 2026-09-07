// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSRFTokenFromContext(t *testing.T) {
	t.Parallel()

	ctx := ContextWithCSRFToken(t.Context(), "secret-token")
	assert.Equal(t, "secret-token", CSRFToken(ctx))
	assert.Empty(t, CSRFToken(t.Context()))
}

func TestCSRFHeaderJSON(t *testing.T) {
	t.Parallel()

	assert.Empty(t, CSRFHeaderJSON(t.Context()))

	ctx := ContextWithCSRFToken(t.Context(), "abc")
	assert.JSONEq(t, `{"X-Csrf-Token":"abc"}`, CSRFHeaderJSON(ctx))
	require.NotEmpty(t, CSRFHeaderJSON(ctx))
}
