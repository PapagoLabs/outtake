// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package metadata

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestPrintDefaultReportsTheWriteFailure(t *testing.T) {
	t.Parallel()

	err := PrintDefault(failingWriter{})

	require.ErrorIs(t, err, errWriteFailed)
	require.ErrorContains(t, err, "write default output")
}

func TestPrintVerboseReportsTheWriteFailure(t *testing.T) {
	t.Parallel()

	err := PrintVerbose(failingWriter{})

	require.ErrorIs(t, err, errWriteFailed)
	require.ErrorContains(t, err, "write verbose output")
}

func TestPrintJSONReportsTheEncodeFailure(t *testing.T) {
	t.Parallel()

	err := PrintJSON(failingWriter{})

	require.ErrorIs(t, err, errWriteFailed)
	require.ErrorContains(t, err, "encode json")
}

//nolint:paralleltest // mutates package-level version state.
func TestPrintVerboseIncludesTheBuildDetails(t *testing.T) {
	restoreVersionState(t)

	ensureVersion()

	CommitSHA = "def5678"
	BuildTime = "2026-01-02T03:04:05Z"

	var buf bytes.Buffer

	require.NoError(t, PrintVerbose(&buf))

	assert.Contains(t, buf.String(), "commitSha: def5678")
	assert.Contains(t, buf.String(), "buildTime: "+ConvertToLocal("2026-01-02T03:04:05Z"))
}

//nolint:paralleltest // mutates package-level version state.
func TestPrintVerboseOmitsTheBuildDetailsWhenTheyAreEmpty(t *testing.T) {
	restoreVersionState(t)

	ensureVersion()

	CommitSHA = ""
	BuildTime = ""

	var buf bytes.Buffer

	require.NoError(t, PrintVerbose(&buf))

	assert.NotContains(t, buf.String(), "commitSha:")
	assert.NotContains(t, buf.String(), "buildTime:")
}
