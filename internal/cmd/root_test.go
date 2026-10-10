// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/metadata"
)

// errRunFailed is the failure the probe command returns while running.
var errRunFailed = errors.New("run failed")

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
	assert.Contains(t, stdout, "Manage the Plex account that owns outtake")
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

// TestSilenceUsageOnRunErrors covers when the usage text is printed: a
// mistyped flag still shows it, and an error the command returns while
// running does not.
func TestSilenceUsageOnRunErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantErr   error
		wantUsage bool
	}{
		{name: "run error", args: []string{"probe"}, wantErr: errRunFailed, wantUsage: false},
		{name: "unknown flag", args: []string{"probe", "--nope"}, wantErr: nil, wantUsage: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := &cobra.Command{Use: "outtake"}
			root.AddCommand(&cobra.Command{
				Use: "probe",
				RunE: func(*cobra.Command, []string) error {
					return errRunFailed
				},
			})

			silenceUsageOnRunErrors(root)

			var output bytes.Buffer

			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(test.args)

			err := root.Execute()
			require.Error(t, err)

			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			}

			assert.Equal(t, test.wantUsage, strings.Contains(output.String(), "Usage:"))
		})
	}
}
