// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCSRFFormFieldName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "_csrf", CSRFFormField)
}

func TestContextWithCSRFTokenIsReadableByCSRFToken(t *testing.T) {
	t.Parallel()

	ctx := ContextWithCSRFToken(t.Context(), "csrf-token-value")

	assert.Equal(t, "csrf-token-value", CSRFToken(ctx))
}

func TestCSRFTokenWithoutAValue(t *testing.T) {
	t.Parallel()

	assert.Empty(t, CSRFToken(t.Context()))
}

func TestCSRFHeaderJSONQuotesTheToken(t *testing.T) {
	t.Parallel()

	ctx := ContextWithCSRFToken(t.Context(), "csrf-token-value")

	assert.JSONEq(t, `{"X-Csrf-Token":"csrf-token-value"}`, CSRFHeaderJSON(ctx))
}

func TestCSRFHeaderJSONWithoutAToken(t *testing.T) {
	t.Parallel()

	assert.Empty(t, CSRFHeaderJSON(t.Context()))
}

func TestCSRFHeaderJSONWithAnEmptyToken(t *testing.T) {
	t.Parallel()

	ctx := ContextWithCSRFToken(t.Context(), "")

	assert.Empty(t, CSRFHeaderJSON(ctx))
}
