// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package owner

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/store/database"
)

// useDatabase points the configuration at a fresh SQLite file.
//
// Parameters:
//   - t: The test that owns the database.
//
// Returns:
//   - path: The database file the command opens.
func useDatabase(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "outtake.db")

	t.Setenv("OUTTAKE_DATABASE_PATH", path)
	t.Setenv("OUTTAKE_STORAGE_PATH", filepath.Join(dir, "output"))

	return path
}

func TestNewCommandOffersReset(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()

	assert.Equal(t, "owner", cmd.Use)
	require.Len(t, cmd.Commands(), 1)
	assert.Equal(t, "reset", cmd.Commands()[0].Use)
}

//nolint:paralleltest // Points the configuration at a database through the environment.
func TestResetForgetsTheOwner(t *testing.T) {
	path := useDatabase(t)

	db, err := database.New(path)
	require.NoError(t, err)

	claimed, err := db.ClaimOwner(t.Context(), 42, "nick")
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, db.Close())

	var out bytes.Buffer

	cmd := NewCommand()
	cmd.SetArgs([]string{"reset"})
	cmd.SetOut(&out)

	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.Equal(t, msgReset, out.String())

	db, err = database.New(path)
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	owned, err := db.HasOwner(t.Context())
	require.NoError(t, err)
	assert.False(t, owned)
}

//nolint:paralleltest // Points the configuration at a database through the environment.
func TestResetReportsThatThereWasNoOwner(t *testing.T) {
	useDatabase(t)

	var out bytes.Buffer

	require.NoError(t, runReset(&out))
	assert.Equal(t, msgNoOwner, out.String())
}

func TestResetReportsAConfigFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	t.Setenv("OUTTAKE_DATABASE_PATH", filepath.Join(blocker, "outtake.db"))

	err := runReset(&bytes.Buffer{})

	require.ErrorContains(t, err, "load config")
}

func TestResetReportsADatabaseFailure(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("OUTTAKE_DATABASE_PATH", filepath.Join(dir, "outtake.db"))
	t.Setenv("OUTTAKE_STORAGE_PATH", filepath.Join(dir, "output"))
	t.Setenv("OUTTAKE_DATABASE_BACKEND", "unknown")

	err := runReset(&bytes.Buffer{})

	require.ErrorContains(t, err, "open database")
}
