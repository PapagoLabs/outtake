// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/settings/flags"
)

// blockedDatabasePath points the database at a parent that cannot be created.
//
// Parameters:
//   - t: The test requesting the path.
//
// Returns:
//   - path: A database file nested under a regular file.
func blockedDatabasePath(t *testing.T) string {
	t.Helper()

	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	return filepath.Join(blocker, "outtake.db")
}

// isolateConfig points the configuration at throwaway directories and a dead port.
//
// Parameters:
//   - t: The test requesting the isolation.
func isolateConfig(t *testing.T) {
	t.Helper()

	dir := t.TempDir()

	t.Setenv("OUTTAKE_DATABASE_PATH", filepath.Join(dir, "outtake.db"))
	t.Setenv("OUTTAKE_STORAGE_PATH", filepath.Join(dir, "output"))
	t.Setenv("OUTTAKE_LISTEN_ADDR", "127.0.0.1:1")
}

func TestNewStartCommand(t *testing.T) {
	t.Parallel()

	cmd := NewStartCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "start", cmd.Use)
	assert.Equal(t, "Start the outtake server", cmd.Short)
	assert.Empty(t, cmd.Commands())
	require.NotNil(t, cmd.RunE)

	assert.NotNil(t, cmd.Flags().Lookup(flags.FlagListen))
}

func TestRunStartWrapsTheConfigFailure(t *testing.T) {
	t.Setenv("OUTTAKE_DATABASE_PATH", blockedDatabasePath(t))

	err := runStart(&flags.Listen{})

	require.Error(t, err)
	require.ErrorContains(t, err, "load config:")
}

func TestRunStartWrapsTheAppFailure(t *testing.T) {
	isolateConfig(t)
	t.Setenv("OUTTAKE_DATABASE_BACKEND", "not-a-database-backend")

	err := runStart(&flags.Listen{Addr: "127.0.0.1:2"})

	require.Error(t, err)
	require.ErrorContains(t, err, "init app:")
	require.ErrorContains(t, err, "not-a-database-backend")
}

func TestNewStartCommandRunEReportsTheConfigFailure(t *testing.T) {
	t.Setenv("OUTTAKE_DATABASE_PATH", blockedDatabasePath(t))

	cmd := NewStartCommand()

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.SetArgs([]string{})

	err := cmd.Execute()

	require.Error(t, err)
	require.ErrorContains(t, err, "load config:")
}
