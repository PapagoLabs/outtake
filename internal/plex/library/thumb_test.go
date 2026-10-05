// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheID(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"e7e69e4a12e8585ff807d0cd0bd8e30d1f76bd696d62d8d90d4ec111d8f64bf4",
		CacheID("/library/metadata/12345/thumb/54321"),
	)
}

func TestCacheIDIsStablePerPath(t *testing.T) {
	t.Parallel()

	first := CacheID("/thumb/a")
	second := CacheID("/thumb/a")

	assert.Equal(
		t,
		"8fa2164104e1729142925cd3cbc68e854ef34d57d97c34763b455bd63eb311d0",
		first,
	)
	assert.Regexp(t, `^[0-9a-f]{64}$`, second)
	assert.NotEqual(t, first, CacheID("/thumb/b"))
}

func TestCacheControl(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "public, max-age=604800, immutable", CacheControl)
}
