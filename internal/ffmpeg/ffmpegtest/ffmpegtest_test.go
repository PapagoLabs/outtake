// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpegtest

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// result is what one run of a fake produced.
type result struct {
	// stdout is what the fake wrote to standard output.
	stdout string
	// stderr is what the fake wrote to standard error.
	stderr string
	// code is the status the fake exited with.
	code int
}

func TestMain(m *testing.M) {
	Dispatch()

	os.Exit(m.Run())
}

// runStub starts a fake and waits for it.
//
// Parameters:
//   - t: The test that runs the fake.
//   - path: The fake to run.
//   - args: Arguments to start it with.
//
// Returns:
//   - got: Its output and exit status.
func runStub(t *testing.T, path string, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(t.Context(), path, args...)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	code := 0

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		code = exitErr.ExitCode()
	} else {
		require.NoError(t, err)
	}

	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func TestInstalledStubWritesItsOutputAndExits(t *testing.T) {
	t.Parallel()

	path := Install(t, Stub{Stdout: "out\n", Stderr: "err\n", ExitCode: 3})

	assert.Equal(t, result{stdout: "out\n", stderr: "err\n", code: 3}, runStub(t, path, "-i", "x"))
}

func TestInstalledStubRecordsItsArguments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	log := filepath.Join(dir, "argv.log")
	output := filepath.Join(dir, "out.mp4")

	path := Install(t, Stub{
		ArgvFile:         log,
		ArgvSeparator:    "----",
		ArgvBesideOutput: true,
		Output:           "encoded",
	})

	runStub(t, path, "-i", "in.mkv", output)
	runStub(t, path, "-y", output)

	logged, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, "-i\nin.mkv\n"+output+"\n----\n-y\n"+output+"\n----\n", string(logged))

	beside, err := os.ReadFile(output + ".argv")
	require.NoError(t, err)
	assert.Equal(t, "-i\nin.mkv\n"+output+"\n-y\n"+output+"\n", string(beside))

	written, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.Equal(t, "encoded", string(written))
}

func TestInstalledStubLeavesStdoutTargetsAlone(t *testing.T) {
	t.Parallel()

	path := Install(t, Stub{ArgvBesideOutput: true, Output: "encoded", Stdout: "piped"})

	assert.Equal(
		t,
		result{stdout: "piped", stderr: "", code: 0},
		runStub(t, path, "-f", "null", "-"),
	)
}

func TestInstalledStubSwapsTheNamedFile(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		swap Swap
		args func(target string) []string
	}{
		{name: "input", swap: SwapInput, args: func(target string) []string { return []string{"-i", target, "-"} }},
		{name: "last", swap: SwapLast, args: func(target string) []string { return []string{"-show_format", target} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			target := filepath.Join(dir, "source.mkv")

			require.NoError(t, os.WriteFile(target, []byte("original"), 0o600))
			require.NoError(
				t,
				os.WriteFile(filepath.Join(dir, "replacement.mkv"), []byte("swapped"), 0o600),
			)

			got := runStub(t, Install(t, Stub{Swap: test.swap}), test.args(target)...)
			require.Zero(t, got.code, got.stderr)

			content, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, "swapped", string(content))
		})
	}
}

func TestInstalledStubReportsAFailedSwap(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "source.mkv")

	got := runStub(t, Install(t, Stub{Swap: SwapInput}), "-i", target)

	assert.Equal(t, failureExit, got.code)
	assert.Contains(t, got.stderr, "ffmpegtest: swap")
}

func TestInstalledStubsRunWhileOthersAreInstalled(t *testing.T) {
	t.Parallel()

	const fakes = 32

	var (
		group sync.WaitGroup
		codes [fakes]int
	)

	for index := range fakes {
		group.Go(func() {
			path := Install(t, Stub{ExitCode: 7})

			cmd := exec.CommandContext(t.Context(), path)

			if exitErr, ok := errors.AsType[*exec.ExitError](cmd.Run()); ok {
				codes[index] = exitErr.ExitCode()
			}
		})
	}

	group.Wait()

	for _, code := range codes {
		assert.Equal(t, 7, code, "every fake ran")
	}
}
