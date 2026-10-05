// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package identity

import (
	"context"
	"strconv"
)

// csrfKey is the context key for a CSRF token stored by middleware.
type csrfKey struct{}

// CSRFFormField is the hidden form field name for Fiber CSRF tokens.
const CSRFFormField = "_csrf"

// ContextWithCSRFToken stores a CSRF token on ctx for templates.
//
// Parameters:
//   - parent: Parent context.
//   - token: CSRF token to store.
//
// Returns:
//   - ctx: Derived context that carries the token.
func ContextWithCSRFToken(parent context.Context, token string) context.Context {
	return context.WithValue(parent, csrfKey{}, token)
}

// CSRFToken returns the CSRF token stored on ctx.
//
// Parameters:
//   - ctx: Request context, optionally carrying a token from middleware.
//
// Returns:
//   - token: The stored token, or empty when none is present.
func CSRFToken(ctx context.Context) string {
	token, ok := ctx.Value(csrfKey{}).(string)
	if !ok {
		return ""
	}

	return token
}

// CSRFHeaderJSON returns hx-headers JSON for the CSRF header.
//
// Parameters:
//   - ctx: Request context that may carry a CSRF token.
//
// Returns:
//   - json: Object mapping X-Csrf-Token, or empty when no token is set.
func CSRFHeaderJSON(ctx context.Context) string {
	token := CSRFToken(ctx)
	if token == "" {
		return ""
	}

	return `{"X-Csrf-Token":` + strconv.Quote(token) + `}`
}
