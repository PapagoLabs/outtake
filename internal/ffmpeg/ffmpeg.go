// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"errors"
	"time"
)

// ExecFFmpeg runs the ffmpeg and ffprobe binaries.
type ExecFFmpeg struct {
	ffmpegPath  string
	ffprobePath string
	timeout     time.Duration
}

const (
	// minTimeout is the shortest deadline a run that scales with its clip gets.
	minTimeout = 30 * time.Minute

	// timeoutScale is how many times a clip's length a run that scales with
	// its clip may take. A 4K encode on a slow CPU runs many times slower than
	// the clip plays.
	timeoutScale = 20

	// waitDelay bounds how long a killed ffmpeg may hold its output open before
	// the run returns anyway.
	waitDelay = 10 * time.Second
)

// ErrTimeout reports an ffmpeg run that was stopped at its deadline.
var ErrTimeout = errors.New("ffmpeg ran past its time limit")

// NewExecFFmpeg creates a new FFmpeg executor whose deadline scales with the
// length of each clip.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - ffprobePath: Path to the ffprobe binary.
//
// Returns:
//   - exec: The executor.
func NewExecFFmpeg(ffmpegPath, ffprobePath string) *ExecFFmpeg {
	return &ExecFFmpeg{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		timeout:     0,
	}
}

// WithTimeout returns a copy of the executor that gives every run the same
// deadline.
//
// Parameters:
//   - timeout: The deadline for each run, or zero or less to scale it with
//     the clip.
//
// Returns:
//   - exec: The configured copy.
func (execFFmpeg *ExecFFmpeg) WithTimeout(timeout time.Duration) *ExecFFmpeg {
	configured := *execFFmpeg

	configured.timeout = max(timeout, 0)

	return &configured
}

// deadline returns how long one run may take.
//
// Parameters:
//   - duration: Length of the clip the run renders, zero when unknown.
//
// Returns:
//   - limit: The configured deadline, otherwise the larger of minTimeout and
//     timeoutScale times the clip length.
func (execFFmpeg *ExecFFmpeg) deadline(duration time.Duration) time.Duration {
	if execFFmpeg.timeout > 0 {
		return execFFmpeg.timeout
	}

	return max(minTimeout, timeoutScale*duration)
}
