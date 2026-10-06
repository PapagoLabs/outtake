// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"strings"
	"testing"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
)

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

// probeSwapStub returns an ffprobe fake that swaps the probed file before it
// prints payload, so a cache keyed on the pre-probe identity must be rejected.
//
// Parameters:
//   - t: The test the fake belongs to.
//   - payload: The ffprobe JSON.
//
// Returns:
//   - path: The fake ffprobe.
func probeSwapStub(t *testing.T, payload string) string {
	t.Helper()

	return ffmpegtest.Install(
		t,
		ffmpegtest.Stub{Stdout: terminated(payload), Swap: ffmpegtest.SwapLast},
	)
}

// terminated ends payload with a newline, as a tool's output does.
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
