// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"context"

	"github.com/PapagoLabs/outtake/internal/media/progress"
)

// WithProgress attaches a progress callback to the context.
//
// Parameters:
//   - ctx: Parent context.
//   - fn: Callback that receives a 0-99 percent complete value.
//
// Returns:
//   - ctx: A child context that carries fn.
func WithProgress(ctx context.Context, fn func(percent int)) context.Context {
	return progress.WithProgress(ctx, fn)
}
