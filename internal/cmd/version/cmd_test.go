// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package version

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/metadata"
	"github.com/PapagoLabs/outtake/internal/settings/flags"
)

// failingWriter is a writer whose every write fails.
type failingWriter struct{}

// errWriteFailed is the failure every failingWriter reports.
var errWriteFailed = errors.New("write failed")

// Write reports the write failure without consuming anything.
//
// Parameters:
//   - p: The bytes the caller wanted written, which are discarded.
//
// Returns:
//   - written: Always zero, because nothing was accepted.
//   - err: Always errWriteFailed.
func (failingWriter) Write(p []byte) (int, error) {
	return 0, errWriteFailed
}

// assertDefaultOutput checks the single-line default printer output.
func assertDefaultOutput(t *testing.T, out string) {
	t.Helper()

	assert.Equal(t, metadata.Name+" "+metadata.String()+"\n", out)
}

// assertVerboseOutput checks the multi-line verbose printer output.
func assertVerboseOutput(t *testing.T, out string) {
	t.Helper()

	assert.Contains(t, out, "name:      "+metadata.Name)
	assert.Contains(t, out, "version:   "+metadata.Version)
	assert.Contains(t, out, "goVersion: ")
}

// assertJSONOutput checks the JSON printer output decodes into version info.
func assertJSONOutput(t *testing.T, out string) {
	t.Helper()

	var info metadata.VersionInfo

	require.NoError(t, json.Unmarshal([]byte(out), &info))
	assert.Equal(t, metadata.Name, info.Name)
	assert.Equal(t, metadata.Version, info.Version)
}

// newBufferedCommand returns a command that captures both of its streams.
//
// Returns:
//   - cmd: The command with its streams captured.
//   - buf: The buffer holding everything the command wrote.
func newBufferedCommand() (*cobra.Command, *bytes.Buffer) {
	var buf bytes.Buffer

	cmd := &cobra.Command{}

	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	return cmd, &buf
}

// newExecutableCommand returns a version command bound to the given arguments.
//
// Parameters:
//   - args: The arguments cobra parses, which must not be nil so cobra does
//     not fall back to the test binary arguments.
//
// Returns:
//   - cmd: The version command with its streams captured.
//   - buf: The buffer holding everything the command wrote.
func newExecutableCommand(args []string) (*cobra.Command, *bytes.Buffer) {
	var buf bytes.Buffer

	cmd := NewCommand()

	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.SetArgs(args)

	return cmd, &buf
}

func TestSelectPrinter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		give  flags.Version
		check func(t *testing.T, out string)
	}{
		{
			name:  "no flag selects the default printer",
			give:  flags.Version{},
			check: assertDefaultOutput,
		},
		{
			name:  "verbose selects the verbose printer",
			give:  flags.Version{Verbose: true},
			check: assertVerboseOutput,
		},
		{
			name:  "json selects the json printer",
			give:  flags.Version{JSON: true},
			check: assertJSONOutput,
		},
		{
			name:  "json wins over verbose",
			give:  flags.Version{JSON: true, Verbose: true},
			check: assertJSONOutput,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			printer := selectPrinter(&test.give)

			require.NoError(t, printer(&buf))
			test.check(t, buf.String())
		})
	}
}

func TestRunVersionCmdWritesTheDefaultOutput(t *testing.T) {
	t.Parallel()

	cmd, buf := newBufferedCommand()

	require.NoError(t, runVersionCmd(cmd, &flags.Version{}))

	assertDefaultOutput(t, buf.String())
}

func TestRunVersionCmdWritesTheVerboseOutput(t *testing.T) {
	t.Parallel()

	cmd, buf := newBufferedCommand()

	require.NoError(t, runVersionCmd(cmd, &flags.Version{Verbose: true}))

	assertVerboseOutput(t, buf.String())
}

func TestRunVersionCmdWritesTheJSONOutput(t *testing.T) {
	t.Parallel()

	cmd, buf := newBufferedCommand()

	require.NoError(t, runVersionCmd(cmd, &flags.Version{JSON: true}))

	assertJSONOutput(t, buf.String())
}

func TestRunVersionCmdFallsBackToStandardOutput(t *testing.T) {
	t.Parallel()

	require.NoError(t, runVersionCmd(&cobra.Command{}, &flags.Version{}))
}

func TestRunVersionCmdWrapsThePrinterFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give flags.Version
	}{
		{name: "the default printer fails", give: flags.Version{}},
		{name: "the verbose printer fails", give: flags.Version{Verbose: true}},
		{name: "the json printer fails", give: flags.Version{JSON: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{}

			cmd.SetOut(failingWriter{})

			err := runVersionCmd(cmd, &test.give)

			require.ErrorIs(t, err, errWriteFailed)
			require.ErrorContains(t, err, "print version")
		})
	}
}

func TestNewCommand(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "version", cmd.Use)
	assert.Equal(t, "Print the application version", cmd.Short)
	assert.Equal(
		t, "Print the application version, including commit SHA and build details.", cmd.Long,
	)
	assert.Contains(t, cmd.Example, "outtake version --json")
	assert.Empty(t, cmd.Commands())
	require.NotNil(t, cmd.RunE)

	assert.NotNil(t, cmd.Flags().Lookup(flags.FlagJSON))
	assert.NotNil(t, cmd.Flags().Lookup(flags.FlagVerbose))
}

func TestNewCommandRunEWritesTheDefaultOutput(t *testing.T) {
	t.Parallel()

	cmd, buf := newExecutableCommand([]string{})

	require.NoError(t, cmd.Execute())

	assertDefaultOutput(t, buf.String())
}

func TestNewCommandRunEWritesTheVerboseOutput(t *testing.T) {
	t.Parallel()

	cmd, buf := newExecutableCommand([]string{"--" + flags.FlagVerbose})

	require.NoError(t, cmd.Execute())

	assertVerboseOutput(t, buf.String())
}

func TestNewCommandRunEWritesTheJSONOutput(t *testing.T) {
	t.Parallel()

	cmd, buf := newExecutableCommand([]string{"--" + flags.FlagJSON})

	require.NoError(t, cmd.Execute())

	assertJSONOutput(t, buf.String())
}

func TestNewCommandRunEReportsThePrinterFailure(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()

	cmd.SetOut(failingWriter{})
	cmd.SetErr(failingWriter{})

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.SetArgs([]string{})

	err := cmd.Execute()

	require.ErrorIs(t, err, errWriteFailed)
	require.ErrorContains(t, err, "print version")
}

func TestNewCommandRunERejectsAnUnknownFlag(t *testing.T) {
	t.Parallel()

	cmd, _ := newExecutableCommand([]string{"--nope"})

	err := cmd.Execute()

	require.Error(t, err)
	require.ErrorContains(t, err, "unknown flag")
}

func TestNewCommandBuildsIsolatedFlagSets(t *testing.T) {
	t.Parallel()

	first := NewCommand()
	second := NewCommand()

	require.NoError(t, first.Flags().Set(flags.FlagJSON, "true"))

	assert.Equal(t, "false", second.Flags().Lookup(flags.FlagJSON).Value.String())
}
