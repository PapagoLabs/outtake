// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package ffmpegtest provides fake ffmpeg and ffprobe binaries for tests.
//
// A fake is the running test binary itself, reached through a symlink named
// after the fake. Install writes what the fake does to a data file beside the
// symlink, and Dispatch, called first in the package's TestMain, acts it out
// when the binary starts under that name. No executable file is ever written,
// so running a fake can never fail with "text file busy".
package ffmpegtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// Swap names the argument whose file a fake replaces while it runs.
type Swap string

// Stub is what a fake binary does when it runs.
type Stub struct {
	// Stdout is written to standard output.
	Stdout string `json:"stdout"`

	// Stderr is written to standard error.
	Stderr string `json:"stderr"`

	// ExitCode is the status the fake exits with.
	ExitCode int `json:"exitCode"`

	// Swap names a file the fake replaces with replacement.mkv from the same
	// directory before it writes anything, so a cache keyed on the file's
	// identity sees it change mid-run.
	Swap Swap `json:"swap"`

	// ArgvFile, when set, receives every argument on its own line.
	ArgvFile string `json:"argvFile"`

	// ArgvSeparator, when set, is appended to ArgvFile after the arguments, so
	// several runs can be told apart.
	ArgvSeparator string `json:"argvSeparator"`

	// ArgvBesideOutput appends every argument to "<last argument>.argv".
	ArgvBesideOutput bool `json:"argvBesideOutput"`

	// Output, when set, is written to the file named by the last argument.
	Output string `json:"output"`
}

const (
	// SwapNone leaves every file alone.
	SwapNone Swap = ""

	// SwapInput replaces the file after the "-i" argument, as ffmpeg reads it.
	SwapInput Swap = "input"

	// SwapLast replaces the file named by the last argument, as ffprobe reads it.
	SwapLast Swap = "last"
)

const (
	// specSuffix names the data file beside a fake's symlink.
	specSuffix = ".json"

	// binaryName is the file name of every fake's symlink.
	binaryName = "ffmpeg-stub"

	// stdoutTarget is the output argument that means "write to stdout".
	stdoutTarget = "-"

	// newline ends every recorded line.
	newline = "\n"

	// fileMode is the permission of every file a fake or Install writes.
	fileMode = 0o600

	// failureExit is the status a fake exits with when it cannot act.
	failureExit = 125
)

// Install creates a fake binary for the length of a test.
//
// Parameters:
//   - tb: The test the fake belongs to.
//   - stub: What the fake does.
//
// Returns:
//   - path: The fake to hand to the code under test.
func Install(tb testing.TB, stub Stub) string {
	tb.Helper()

	self, err := os.Executable()
	require.NoError(tb, err, "find the test binary")

	path := filepath.Join(tb.TempDir(), binaryName)

	//nolint:errchkjson // Checked anyway, so a field added later that cannot encode fails the test.
	spec, err := json.Marshal(stub)
	require.NoError(tb, err, "encode the stub")

	require.NoError(tb, os.WriteFile(path+specSuffix, spec, fileMode), "write the stub")
	require.NoError(tb, os.Symlink(self, path), "link the stub to the test binary")

	return path
}

// Dispatch acts out a fake when the test binary started as one, and exits.
// Otherwise it returns at once. Call it first in TestMain.
func Dispatch() {
	//nolint:gosec // The spec sits beside the symlink the test binary was started through.
	spec, err := os.ReadFile(os.Args[0] + specSuffix)
	if err != nil {
		return
	}

	var stub Stub

	err = json.Unmarshal(spec, &stub)
	if err != nil {
		fail(fmt.Errorf("decode stub: %w", err))
	}

	err = run(stub, os.Args[1:])
	if err != nil {
		fail(err)
	}

	exit(stub.ExitCode)
}

// run performs everything a fake does except exiting.
//
// Parameters:
//   - stub: What the fake does.
//   - args: The arguments the fake was started with.
//
// Returns:
//   - err: The first file or write failure.
func run(stub Stub, args []string) error {
	err := recordArgs(stub, args)
	if err != nil {
		return fmt.Errorf("record args: %w", err)
	}

	err = swap(stub.Swap, args)
	if err != nil {
		return fmt.Errorf("swap: %w", err)
	}

	err = writeOutput(stub, args)
	if err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	err = writeStreams(stub)
	if err != nil {
		return fmt.Errorf("write streams: %w", err)
	}

	return nil
}

// recordArgs writes the arguments where the stub asks for them.
//
// Parameters:
//   - stub: What the fake does.
//   - args: The arguments the fake was started with.
//
// Returns:
//   - err: Wrapped write failure.
func recordArgs(stub Stub, args []string) error {
	lines := argLines(args)

	if stub.ArgvFile != "" {
		err := appendFile(stub.ArgvFile, lines+separatorLine(stub.ArgvSeparator))
		if err != nil {
			return fmt.Errorf("argv file: %w", err)
		}
	}

	output := fileTarget(args)
	if !stub.ArgvBesideOutput || output == "" {
		return nil
	}

	err := appendFile(output+".argv", lines)
	if err != nil {
		return fmt.Errorf("argv beside output: %w", err)
	}

	return nil
}

// separatorLine renders the line that ends one run in an argv file.
//
// Parameters:
//   - separator: The separator, or empty for none.
//
// Returns:
//   - line: The separator and a newline, or empty.
func separatorLine(separator string) string {
	if separator == "" {
		return ""
	}

	return separator + newline
}

// swap replaces the file an argument names with replacement.mkv beside it.
//
// Parameters:
//   - mode: Which argument names the file.
//   - args: The arguments the fake was started with.
//
// Returns:
//   - err: Wrapped rename failure.
func swap(mode Swap, args []string) error {
	var target string

	switch mode {
	case SwapInput:
		target = inputArg(args)
	case SwapLast:
		target = lastArg(args)
	default:
		return nil
	}

	if target == "" {
		return nil
	}

	//nolint:gosec // A fake renames the files its own test named.
	err := os.Rename(filepath.Join(filepath.Dir(target), "replacement.mkv"), target)
	if err != nil {
		return fmt.Errorf("rename %s: %w", target, err)
	}

	return nil
}

// writeOutput writes the stub's output to the file the last argument names.
//
// Parameters:
//   - stub: What the fake does.
//   - args: The arguments the fake was started with.
//
// Returns:
//   - err: Wrapped write failure.
func writeOutput(stub Stub, args []string) error {
	output := fileTarget(args)
	if stub.Output == "" || output == "" {
		return nil
	}

	//nolint:gosec // A fake writes the file its own test named.
	err := os.WriteFile(output, []byte(stub.Output), fileMode)
	if err != nil {
		return fmt.Errorf("write %s: %w", output, err)
	}

	return nil
}

// writeStreams writes the stub's standard output and standard error.
//
// Parameters:
//   - stub: What the fake does.
//
// Returns:
//   - err: Wrapped write failure.
func writeStreams(stub Stub) error {
	_, err := os.Stdout.WriteString(stub.Stdout)
	if err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}

	_, err = os.Stderr.WriteString(stub.Stderr)
	if err != nil {
		return fmt.Errorf("write stderr: %w", err)
	}

	return nil
}

// appendFile appends text to a file, creating it when needed.
//
// Parameters:
//   - path: File to append to.
//   - text: Text to append.
//
// Returns:
//   - err: Wrapped open or write failure.
func appendFile(path, text string) error {
	//nolint:gosec // A fake appends to the files its own test named.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	_, err = file.WriteString(text)

	closeErr := file.Close()
	if err != nil {
		return fmt.Errorf("append %s: %w", path, err)
	}

	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}

	return nil
}

// argLines joins arguments one per line.
//
// Parameters:
//   - args: The arguments the fake was started with.
//
// Returns:
//   - lines: Every argument followed by a newline.
func argLines(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return strings.Join(args, newline) + newline
}

// fileTarget returns the last argument when it names a file.
//
// Parameters:
//   - args: The arguments the fake was started with.
//
// Returns:
//   - path: The output file, or empty when the output is stdout or missing.
func fileTarget(args []string) string {
	last := lastArg(args)
	if last == stdoutTarget {
		return ""
	}

	return last
}

// inputArg returns the argument after the first "-i".
//
// Parameters:
//   - args: The arguments the fake was started with.
//
// Returns:
//   - input: The input path, or empty when there is none.
func inputArg(args []string) string {
	for index, arg := range args[:max(len(args)-1, 0)] {
		if arg == "-i" {
			return args[index+1]
		}
	}

	return ""
}

// lastArg returns the last argument.
//
// Parameters:
//   - args: The arguments the fake was started with.
//
// Returns:
//   - last: The last argument, or empty when there are none.
func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return args[len(args)-1]
}

// fail reports why a fake could not act and exits with failureExit.
//
// Parameters:
//   - err: The failure.
func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "ffmpegtest:", err)

	exit(failureExit)
}

// exit ends a fake at once. [os.Exit] would run the race detector's exit hook,
// which sleeps a second in every race-enabled test binary. A fake writes its
// output unbuffered, so there is nothing left to flush.
//
// Parameters:
//   - code: The status to exit with.
func exit(code int) {
	//nolint:revive // A fake is a whole process, so it ends here rather than in main.
	syscall.Exit(code)
}
