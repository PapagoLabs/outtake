// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package health

import (
	"net/http"
	"net/http/httptest"
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

// deadAddr returns the loopback address of a test server that answers
// nothing.
//
// The server closes each request's connection unanswered and holds its port
// until the test ends, so no other listener can answer in its place.
//
// Parameters:
//   - t: The test that needs the address.
//
// Returns:
//   - addr: The server's host and port.
func deadAddr(t *testing.T) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(dropRequest))
	t.Cleanup(server.Close)

	return server.Listener.Addr().String()
}

// dropRequest closes a request's connection without answering it.
//
// Parameters:
//   - writer: The response, whose connection is taken and closed.
func dropRequest(writer http.ResponseWriter, _ *http.Request) {
	conn, _, err := http.NewResponseController(writer).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

func TestNewCommand(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "health", cmd.Use)
	assert.Equal(t, "Check the health of the outtake server", cmd.Short)
	assert.Empty(t, cmd.Commands())
	require.NotNil(t, cmd.RunE)

	assert.NotNil(t, cmd.Flags().Lookup(flags.FlagListen))
}

func TestRunHealthReportsAHealthyServer(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("OUTTAKE_DATABASE_PATH", filepath.Join(dir, "outtake.db"))
	t.Setenv("OUTTAKE_STORAGE_PATH", filepath.Join(dir, "output"))

	ts := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			assert.Equal(t, "/api/healthz", request.URL.Path)

			writer.WriteHeader(http.StatusOK)

			_, _ = writer.Write([]byte("ok"))
		},
	))

	defer ts.Close()

	require.NoError(t, runHealth(&flags.Listen{Addr: ts.Listener.Addr().String()}))
}

func TestRunHealthReportsAnUnhealthyServer(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("OUTTAKE_DATABASE_PATH", filepath.Join(dir, "outtake.db"))
	t.Setenv("OUTTAKE_STORAGE_PATH", filepath.Join(dir, "output"))

	ts := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			assert.Equal(t, "/api/healthz", request.URL.Path)

			writer.WriteHeader(http.StatusServiceUnavailable)
		},
	))

	defer ts.Close()

	err := runHealth(&flags.Listen{Addr: ts.Listener.Addr().String()})

	require.Error(t, err)
	require.ErrorContains(t, err, "health check:")
}

func TestRunHealthReportsAnUnreachableServer(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("OUTTAKE_DATABASE_PATH", filepath.Join(dir, "outtake.db"))
	t.Setenv("OUTTAKE_STORAGE_PATH", filepath.Join(dir, "output"))

	err := runHealth(&flags.Listen{Addr: deadAddr(t)})

	require.Error(t, err)
	require.ErrorContains(t, err, "health check:")
}

func TestRunHealthUsesTheConfiguredAddress(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("OUTTAKE_DATABASE_PATH", filepath.Join(dir, "outtake.db"))
	t.Setenv("OUTTAKE_STORAGE_PATH", filepath.Join(dir, "output"))
	t.Setenv("OUTTAKE_LISTEN_ADDR", deadAddr(t))

	err := runHealth(&flags.Listen{})

	require.Error(t, err)
	require.ErrorContains(t, err, "health check:")
}

func TestRunHealthWrapsTheConfigFailure(t *testing.T) {
	t.Setenv("OUTTAKE_DATABASE_PATH", blockedDatabasePath(t))

	err := runHealth(&flags.Listen{})

	require.Error(t, err)
	require.ErrorContains(t, err, "load config:")
}

func TestNewCommandRunEReportsTheConfigFailure(t *testing.T) {
	t.Setenv("OUTTAKE_DATABASE_PATH", blockedDatabasePath(t))

	cmd := NewCommand()

	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	cmd.SetArgs([]string{})

	err := cmd.Execute()

	require.Error(t, err)
	require.ErrorContains(t, err, "load config:")
}
