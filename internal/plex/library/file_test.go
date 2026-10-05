// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileExists(t *testing.T) {
	t.Parallel()

	assert.False(t, FileExists(""), "an absent path is never a file")
	assert.False(t, FileExists(filepath.Join(t.TempDir(), "missing.mp4")))
	assert.False(t, FileExists(t.TempDir()),
		"a directory is not a rendered artifact")

	path := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(path, []byte("rendered"), 0o600))

	assert.True(t, FileExists(path))
}
