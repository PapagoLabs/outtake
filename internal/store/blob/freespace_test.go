// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build linux || darwin

package blob

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFreeSpaceReadsTheVolume covers the free space line, which this system
// measures.
func TestFreeSpaceReadsTheVolume(t *testing.T) {
	t.Parallel()

	free, total, ok := FreeSpace(t.TempDir())
	require.True(t, ok)
	assert.Positive(t, total)
	assert.LessOrEqual(t, free, total)

	_, _, ok = FreeSpace(filepath.Join(t.TempDir(), "missing"))
	assert.False(t, ok, "a path that is not there cannot be measured")
}
