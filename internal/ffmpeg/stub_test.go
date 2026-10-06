// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"strings"
	"testing"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
)

// passStub returns an ffmpeg fake whose pass writes payload to stderr.
//
// Parameters:
//   - t: The test the fake belongs to.
//   - stderr: What the pass logs.
//
// Returns:
//   - path: The fake ffmpeg.
func passStub(t *testing.T, stderr string) string {
	t.Helper()

	return ffmpegtest.Install(t, ffmpegtest.Stub{Stderr: terminated(stderr)})
}

// failingPassStub returns an ffmpeg fake whose pass logs payload and fails.
//
// Parameters:
//   - t: The test the fake belongs to.
//   - stderr: What the pass logs.
//
// Returns:
//   - path: The fake ffmpeg.
func failingPassStub(t *testing.T, stderr string) string {
	t.Helper()

	return ffmpegtest.Install(t, ffmpegtest.Stub{Stderr: terminated(stderr), ExitCode: 1})
}

// swappingPassStub returns an ffmpeg fake that swaps the source file while the
// pass runs, so a cache keyed on the pre-pass identity must be rejected.
//
// Parameters:
//   - t: The test the fake belongs to.
//   - stderr: What the pass logs.
//
// Returns:
//   - path: The fake ffmpeg.
func swappingPassStub(t *testing.T, stderr string) string {
	t.Helper()

	return ffmpegtest.Install(
		t,
		ffmpegtest.Stub{Stderr: terminated(stderr), Swap: ffmpegtest.SwapInput},
	)
}

// probeStub returns an ffprobe fake that prints payload.
//
// Parameters:
//   - t: The test the fake belongs to.
//   - payload: The ffprobe JSON.
//
// Returns:
//   - path: The fake ffprobe.
func probeStub(t *testing.T, payload string) string {
	t.Helper()

	return ffmpegtest.Install(t, ffmpegtest.Stub{Stdout: terminated(payload)})
}

// terminated ends payload with a newline, as a tool's log line does.
//
// Parameters:
//   - payload: Text to end.
//
// Returns:
//   - text: payload with a trailing newline.
func terminated(payload string) string {
	if strings.HasSuffix(payload, "\n") {
		return payload
	}

	return payload + "\n"
}
