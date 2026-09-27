// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"os"
	"testing"
)

// TestMain writes every ffmpeg stub the media tests use before any test runs.
//
// The stubs stand in for the ffmpeg binary so a code path can be exercised
// without one installed. Writing them here rather than from a test keeps the
// write strictly separated from the exec, which this overlayfs-backed
// filesystem does not guarantee otherwise.
func TestMain(m *testing.M) {
	for _, script := range []string{
		plainStubScript(cropdetectStubLog),
		plainStubScript(signalstatsStubLog),
		plainStubScript("no signalstats output here"),
		swapStubScript(cropdetectStubLog),
		swapStubScript(signalstatsStubLog),
		probeSwapStubScript(stubProbeJSON),
		probeStubScript(stubProbeJSON),
	} {
		writeStubDirect(script)
	}

	os.Exit(m.Run())
}

// writeStubDirect writes one stub script, ignoring an already written one.
//
// Parameters:
//   - script: Full script text, including the shebang.
func writeStubDirect(script string) {
	dir := stubDir()
	if dir == "" {
		return
	}

	path := stubPath(dir, script)

	_, statErr := os.Stat(path)
	if os.IsNotExist(statErr) {
		_ = os.WriteFile(path, []byte(script), 0o700)
	}
}
