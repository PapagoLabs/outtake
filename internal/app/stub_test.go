// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
)

// recordedStages stands in for the queue, recording each stage a render
// reports.
type recordedStages struct {
	// mu guards stages.
	mu sync.Mutex
	// stages are the reported stages, in order.
	stages []clip.Stage
}

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

// extractWithCrop renders a job as the worker does: the crop is detected
// once, then the extract runs with it.
//
// Parameters:
//   - t: The test the render belongs to.
//   - job: The job to render.
//   - runner: The FFmpeg runner, which may be nil for a job that never runs it.
//
// Returns:
//   - err: The extract's error.
func extractWithCrop(t *testing.T, job *clip.Job, runner *ffmpeg.ExecFFmpeg) error {
	t.Helper()

	//nolint:wrapcheck // The test reads the extract's own error.
	return extractJob(t.Context(), job, runner, nil, detectJobCrop(t.Context(), runner, job))
}

// SetStage records a reported stage.
//
// Parameters:
//   - _: The clip id, which the record ignores.
//   - stage: The reported stage.
//
// Returns:
//   - job: Always nil, as for a job the queue no longer has.
func (recorded *recordedStages) SetStage(_ string, stage clip.Stage) *clip.Job {
	recorded.mu.Lock()
	defer recorded.mu.Unlock()

	recorded.stages = append(recorded.stages, stage)

	return nil
}

// reported returns the stages recorded so far.
//
// Returns:
//   - stages: A copy of the recorded stages.
func (recorded *recordedStages) reported() []clip.Stage {
	recorded.mu.Lock()
	defer recorded.mu.Unlock()

	return slices.Clone(recorded.stages)
}
