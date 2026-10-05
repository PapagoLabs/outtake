// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"context"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
)

// Probe probes a media file for information.
//
// Parameters:
//   - ctx: Cancellation and deadline for the probe.
//   - path: Media file path.
//
// Returns:
//   - info: The probed media information.
//   - err: Non-nil when the file cannot be probed.
func (execFFmpeg *ExecFFmpeg) Probe(ctx context.Context, path string) (probe.Info, error) {
	info, err := probe.Probe(ctx, execFFmpeg.ffprobePath, path)
	if err != nil {
		return probe.Info{}, fmt.Errorf("probe media: %w", err)
	}

	return info, nil
}
