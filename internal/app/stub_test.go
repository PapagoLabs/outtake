// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
)

// stubInvocationSeparator ends each run's argv in a fake's log.
const stubInvocationSeparator = "---- stub invocation ----"

func TestMain(m *testing.M) {
	ffmpegtest.Dispatch()

	os.Exit(m.Run())
}

// stubFFmpeg returns an ffmpeg fake that appends the argv of every run to
// logPath and then behaves as stub describes.
//
// Parameters:
//   - t: The test that needs the fake.
//   - logPath: File the fake appends each run's argv to.
//   - stub: What the fake does after recording its argv.
//
// Returns:
//   - path: The fake ffmpeg.
func stubFFmpeg(t *testing.T, logPath string, stub ffmpegtest.Stub) string {
	t.Helper()

	stub.ArgvFile = logPath
	stub.ArgvSeparator = stubInvocationSeparator

	// A render only publishes a file it wrote something to.
	if stub.Output == "" {
		stub.Output = "rendered"
	}

	return ffmpegtest.Install(t, stub)
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

	for block := range strings.SplitSeq(string(raw), stubInvocationSeparator+"\n") {
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

// assertStagedFor checks that a render wrote to a staging file beside output
// and that the staging file was published to output.
//
// Parameters:
//   - t: The test that is checking.
//   - output: The clip's output path.
//   - written: The path the render wrote to.
func assertStagedFor(t *testing.T, output, written string) {
	t.Helper()

	assert.Equal(
		t,
		filepath.Dir(output),
		filepath.Dir(written),
		"the render writes beside the output",
	)
	assert.True(t, strings.HasPrefix(filepath.Base(written), ".staging-"), "to a staging file")
	assert.Equal(t, filepath.Ext(output), filepath.Ext(written), "with the output's extension")
	assert.FileExists(t, output, "which is published once the render succeeds")
	assert.NoFileExists(t, written, "and moved, not copied")
}
