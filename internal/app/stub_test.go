// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

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

// appStubDir is the process wide directory holding the stub binaries.
var appStubDir = sync.OnceValue(func() string {
	dir, err := os.MkdirTemp("", "outtake-app-stub")
	if err != nil {
		return ""
	}

	return dir
})

// stubFFmpeg writes an executable stub that appends the argv of every
// invocation to logPath and then runs body.
//
// Parameters:
//   - t: The test that needs the stub.
//   - logPath: File the stub appends each invocation's argv to.
//   - body: Shell run after the argv has been recorded.
//
// Returns:
//   - path: The stub binary.
func stubFFmpeg(t *testing.T, logPath, body string) string {
	t.Helper()

	_, statErr := os.Stat("/bin/sh")
	if statErr != nil {
		t.Skip("a POSIX shell is required for the ffmpeg stub")
	}

	dir := appStubDir()
	if dir == "" {
		t.Skip("unable to create a directory for the ffmpeg stub")
	}

	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do printf '%s\\n' \"$arg\" >> " + shellQuote(logPath) + "; done\n" +
		"printf '%s\\n' " + shellQuote("---- stub invocation ----") + " >> " + shellQuote(logPath) + "\n" +
		body

	path := stubBinaryPath(dir, script)

	_, pathErr := os.Stat(path)
	if os.IsNotExist(pathErr) {
		require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	}

	return path
}

// missingBinary returns a path that holds no executable.
//
// Parameters:
//   - dir: Directory the path is built inside.
//
// Returns:
//   - path: A path inside dir that no process can run.
func missingBinary(dir string) string {
	return filepath.Join(dir, "no-such-ffmpeg")
}

// stubBinaryPath names the stub file a script is cached under.
//
// Parameters:
//   - dir: Directory the stub is written to.
//   - script: The stub's shell source.
//
// Returns:
//   - path: The content addressed stub path.
func stubBinaryPath(dir, script string) string {
	sum := sha256.Sum256([]byte(script))

	return filepath.Join(dir, "stub-"+hex.EncodeToString(sum[:8]))
}

// stubInputFile creates a source file so a probe can identify it on disk.
//
// Parameters:
//   - t: The test that needs the source.
//   - dir: Directory the file is created in.
//   - name: File name.
//
// Returns:
//   - path: The created file path.
func stubInputFile(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("stub source"), 0o600))

	return path
}

// stubInvocations reads back the argv of every recorded stub invocation. Each
// argv is split on whitespace, so a stub argument must not contain spaces.
//
// Parameters:
//   - t: The test reading the log.
//   - path: File the stub appended to.
//
// Returns:
//   - invocations: One argv slice per invocation, in call order.
func stubInvocations(t *testing.T, path string) [][]string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}

	require.NoError(t, err)

	var invocations [][]string

	for block := range strings.SplitSeq(string(raw), "---- stub invocation ----\n") {
		argv := strings.Fields(block)
		if len(argv) > 0 {
			invocations = append(invocations, argv)
		}
	}

	return invocations
}

// stubArgvContains reports whether any element of an argv carries fragment.
//
// Parameters:
//   - argv: One recorded invocation.
//   - fragment: Substring to look for inside a single argument.
//
// Returns:
//   - found: True when an argument contains the fragment.
func stubArgvContains(argv []string, fragment string) bool {
	for _, arg := range argv {
		if strings.Contains(arg, fragment) {
			return true
		}
	}

	return false
}

// stubOutputArg returns the output path an argv was asked to write.
//
// Parameters:
//   - argv: One recorded invocation.
//
// Returns:
//   - output: The final argv element, empty when the invocation is empty.
func stubOutputArg(argv []string) string {
	if len(argv) == 0 {
		return ""
	}

	return argv[len(argv)-1]
}

// shellQuote renders value as a single POSIX shell word.
//
// Parameters:
//   - value: The word to quote.
//
// Returns:
//   - quoted: The word as a single-quoted shell token.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
