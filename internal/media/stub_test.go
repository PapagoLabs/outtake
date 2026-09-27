// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// swapReplacementName is the file a swap stub moves onto the path under probe.
//
// It is resolved relative to that path by the stub, so a test's script text stays
// identical between tests and can therefore be written once, up front.
const swapReplacementName = "replacement.mkv"

// stubDir holds the stub scripts for the lifetime of the test binary.
var stubDir = sync.OnceValue(func() string {
	dir, err := os.MkdirTemp("", "outtake-stub")
	if err != nil {
		return ""
	}

	return dir
})

// stubScript returns the path of a stub script, writing it if it is not there.
//
// Scripts are written by TestMain before any test runs. Writing one here instead
// would race: this filesystem is overlayfs, where a file that has just been
// closed for writing can still read as busy to a concurrent exec, and that
// surfaces as an intermittent ETXTBSY from a test's own ffmpeg call. Because
// every script is static, nothing has to be written while tests are running.
//
// Parameters:
//   - t: Test context.
//   - script: Full script text, including the shebang.
//
// Returns:
//   - path: Path of the executable script.
func stubScript(t *testing.T, script string) string {
	t.Helper()

	_, statErr := os.Stat("/bin/sh")
	if statErr != nil {
		t.Skip("a POSIX shell is required for the ffmpeg stub")
	}

	dir := stubDir()
	if dir == "" {
		t.Skip("unable to create a directory for the ffmpeg stub")
	}

	path := stubPath(dir, script)

	// TestMain writes the stubs the tests are expected to use. A script it does
	// not know about is written here, which is correct but not race free, so a
	// new stub should be added to that list.
	_, pathErr := os.Stat(path)
	if os.IsNotExist(pathErr) {
		require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	}

	return path
}

// stubPath is the filename a script maps to.
//
// Parameters:
//   - dir: Directory holding the stubs.
//   - script: Full script text.
//
// Returns:
//   - path: Path the script is written to.
func stubPath(dir, script string) string {
	sum := sha256.Sum256([]byte(script))

	return filepath.Join(dir, "stub-"+hex.EncodeToString(sum[:8]))
}

// swapInputLookup locates the input file in an ffmpeg argv.
//
// Ffmpeg writes its argv with the input behind "-i" and the null output last, so
// the final argument is not the file. Reading the wrong argument makes the swap
// a no-op, and the test it guards then passes without exercising anything.
//
// Parameters:
//   - script: Script prefix to place the lookup in.
//
// Returns:
//   - text: Shell fragment assigning the input path to $target.
func swapInputLookup(script string) string {
	return script + "target=\n" +
		"want=0\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$want\" = 1 ]; then target=$arg; break; fi\n" +
		"  if [ \"$arg\" = \"-i\" ]; then want=1; fi\n" +
		"done\n"
}

// swapStubScript builds a stub that replaces the file under analysis with a
// sibling named swapReplacementName before answering on standard error.
//
// The replacement is moved rather than copied, so the file at that path becomes a
// different inode while keeping the same size and modification time. The
// metadata key is therefore unchanged and the file identity is the only thing
// that can tell the two apart, which is what the re-check before caching exists
// to catch.
//
// Parameters:
//   - payload: Text the stub writes to standard error.
//
// Returns:
//   - script: Full script text.
func swapStubScript(payload string) string {
	return swapInputLookup("#!/bin/sh\n") +
		"mv \"${target%/*}/" + swapReplacementName + "\" \"$target\"\n" +
		stderrHeredoc(payload)
}

// plainStubScript builds a stub that answers without touching the file under
// probe.
//
// Parameters:
//   - payload: Text the stub writes to standard error.
//
// Returns:
//   - script: Full script text.
func plainStubScript(payload string) string {
	return "#!/bin/sh\n" + stderrHeredoc(payload)
}

// probeStubScript builds a stub that answers on standard output, which is where
// Probe reads its JSON payload from.
//
// Parameters:
//   - payload: Text the stub writes to standard output.
//
// Returns:
//   - script: Full script text.
func probeStubScript(payload string) string {
	return "#!/bin/sh\n" + stdoutHeredoc(payload)
}

// probeSwapStubScript is probeStubScript, replacing the file under probe first.
//
// The payload stays on standard output, unlike swapStubScript, because Probe
// reads its JSON from there. The file is found by position rather than after
// "-i", because ffprobe is passed the path as a bare argument and never uses an
// input flag.
//
// Parameters:
//   - payload: Text the stub writes to standard output.
//
// Returns:
//   - script: Full script text.
func probeSwapStubScript(payload string) string {
	return "#!/bin/sh\n" +
		"for target; do :; done\n" +
		"mv \"${target%/*}/" + swapReplacementName + "\" \"$target\"\n" +
		stdoutHeredoc(payload)
}

// stdoutHeredoc renders a payload as a standard output heredoc, which is where
// Probe reads its JSON from.
//
// The payload is newline terminated so the terminator cannot be glued onto its
// last line, which would silently truncate what the stub emits.
//
// Parameters:
//   - payload: Text the stub writes.
//
// Returns:
//   - text: A shell heredoc writing payload to standard output.
func stdoutHeredoc(payload string) string {
	return "cat <<'STUB_OUT'\n" + terminated(payload) + "STUB_OUT\n"
}

// stderrHeredoc renders a payload as a standard error heredoc, which is where
// cropdetect and signalstats write their results.
//
// Parameters:
//   - payload: Text the stub writes.
//
// Returns:
//   - text: A shell heredoc writing payload to standard error.
func stderrHeredoc(payload string) string {
	return "cat >&2 <<'STUB_ERR'\n" + terminated(payload) + "STUB_ERR\n"
}

// terminated newline-terminates a payload so a heredoc terminator cannot be
// glued onto its last line.
//
// Parameters:
//   - payload: Text the stub writes.
//
// Returns:
//   - text: payload with a trailing newline.
func terminated(payload string) string {
	if strings.HasSuffix(payload, "\n") {
		return payload
	}

	return payload + "\n"
}
