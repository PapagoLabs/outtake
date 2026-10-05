// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"time"
)

// ExecFFmpeg runs the ffmpeg and ffprobe binaries.
type ExecFFmpeg struct {
	ffmpegPath  string
	ffprobePath string
	timeout     time.Duration
}

// defaultFFmpegTimeout is the deadline applied to a single FFmpeg command.
const defaultFFmpegTimeout = 30 * time.Minute

// NewExecFFmpeg creates a new FFmpeg executor.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - ffprobePath: Path to the ffprobe binary.
//
// Returns:
//   - exec: An executor carrying the default timeout.
func NewExecFFmpeg(ffmpegPath, ffprobePath string) *ExecFFmpeg {
	return &ExecFFmpeg{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		timeout:     DefaultFFmpegTimeout(),
	}
}

// DefaultFFmpegTimeout returns the default FFmpeg timeout.
//
// Returns:
//   - timeout: The per-command deadline.
func DefaultFFmpegTimeout() time.Duration {
	return defaultFFmpegTimeout
}
