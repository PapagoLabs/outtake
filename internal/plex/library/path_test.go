// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/settings/config"
)

func TestLocalMediaFileUnderE2E(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "clip.mkv")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o600))

	got, ok := localMediaFile(config.EnvE2E, file)

	assert.True(t, ok)
	assert.Equal(t, file, got)
}

func TestLocalMediaFileRejectsOtherEnvironments(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "clip.mkv")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o600))

	got, ok := localMediaFile("production", file)

	assert.False(t, ok)
	assert.Empty(t, got)
}

func TestLocalMediaFileRejectsMissingAndDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	got, ok := localMediaFile(config.EnvE2E, filepath.Join(dir, "absent.mkv"))
	assert.False(t, ok)
	assert.Empty(t, got)

	got, ok = localMediaFile(config.EnvE2E, dir)
	assert.False(t, ok)
	assert.Empty(t, got)
}
