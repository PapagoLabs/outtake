// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"context"
)

// managerKey is the context key for whether a page's viewer manages the
// installation.
type managerKey struct{}

// ContextWithManager records whether the viewer manages the installation.
//
// The layout reads it to show the settings only the owner and admins may
// open. The admin guard, not the layout, refuses everyone else.
//
// Parameters:
//   - parent: The request's context.
//   - manages: True for the owner or an admin.
//
// Returns:
//   - ctx: The context carrying the flag.
func ContextWithManager(parent context.Context, manages bool) context.Context {
	return context.WithValue(parent, managerKey{}, manages)
}

// ViewerManages reports whether a page's viewer manages the installation.
//
// Parameters:
//   - ctx: The render's context.
//
// Returns:
//   - manages: True for the owner or an admin, false when nothing recorded it.
func ViewerManages(ctx context.Context) bool {
	manages, ok := ctx.Value(managerKey{}).(bool)

	return ok && manages
}
