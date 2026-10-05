// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/metadata"
)

// executeCLI runs the CLI with the process arguments and streams replaced.
//
// Parameters:
//   - t: The test requesting the capture.
//   - args: The arguments handed to the CLI, without the program name.
//
// Returns:
//   - stdout: Everything the CLI wrote to standard output.
//   - stderr: Everything the CLI wrote to standard error.
//   - err: The error the CLI returned.
func executeCLI(t *testing.T, args []string) (string, string, error) {
	t.Helper()

	outReader, outWriter, err := os.Pipe()
	require.NoError(t, err)

	errReader, errWriter, err := os.Pipe()
	require.NoError(t, err)

	originalArgs := os.Args
	originalStdout := os.Stdout
	originalStderr := os.Stderr

	os.Args = append([]string{"outtake"}, args...)
	os.Stdout = outWriter
	os.Stderr = errWriter

	t.Cleanup(func() {
		os.Args = originalArgs
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	})

	runErr := Execute()

	require.NoError(t, outWriter.Close())
	require.NoError(t, errWriter.Close())

	stdout, readErr := io.ReadAll(outReader)
	require.NoError(t, readErr)

	stderr, readErr := io.ReadAll(errReader)
	require.NoError(t, readErr)

	return strings.TrimSpace(string(stdout)), strings.TrimSpace(string(stderr)), runErr
}

//nolint:paralleltest // Swaps the process arguments and streams, which are process-wide.
func TestExecutePrintsTheRootHelp(t *testing.T) {
	stdout, _, err := executeCLI(t, []string{"--help"})

	require.NoError(t, err)
	assert.Contains(t, stdout, "outtake [command]")
	assert.Contains(t, stdout, "outtake creates video clips")
}

//nolint:paralleltest // Swaps the process arguments and streams, which are process-wide.
func TestExecuteRegistersEverySubcommand(t *testing.T) {
	stdout, _, err := executeCLI(t, []string{"--help"})

	require.NoError(t, err)
	assert.Contains(t, stdout, "Manage the outtake server")
	assert.Contains(t, stdout, "Check the health of the outtake server")
	assert.Contains(t, stdout, "Print the application version")
}

//nolint:paralleltest // Swaps the process arguments and streams, which are process-wide.
func TestExecuteResolvesTheVersionSubcommand(t *testing.T) {
	stdout, _, err := executeCLI(t, []string{"version"})

	require.NoError(t, err)
	assert.Equal(t, metadata.Name+" "+metadata.String(), stdout)
}

//nolint:paralleltest // Swaps the process arguments and streams, which are process-wide.
func TestExecuteResolvesTheServerSubcommand(t *testing.T) {
	stdout, _, err := executeCLI(t, []string{"server", "--help"})

	require.NoError(t, err)
	assert.Contains(t, stdout, "Start the outtake server")
}

//nolint:paralleltest // Swaps the process arguments and streams, which are process-wide.
func TestExecuteWrapsAnUnknownSubcommand(t *testing.T) {
	_, stderr, err := executeCLI(t, []string{"not-a-command"})

	require.Error(t, err)
	require.ErrorContains(t, err, "execute:")
	assert.Contains(t, stderr, "not-a-command")
}
