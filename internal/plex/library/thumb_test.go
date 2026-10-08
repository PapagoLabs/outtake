// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCacheIDIsStableAndKeyedByServer covers the cache key: the same server
// and path always name the same file, and another server or path never does.
func TestCacheIDIsStableAndKeyedByServer(t *testing.T) {
	t.Parallel()

	first := CacheID("machine-1", "/thumb/a")

	assert.Equal(t, first, CacheID("machine-1", "/thumb/a"))
	assert.Regexp(t, `^[0-9a-f]{64}$`, first)
	assert.NotEqual(t, first, CacheID("machine-1", "/thumb/b"), "another path")
	assert.NotEqual(
		t,
		first,
		CacheID("machine-2", "/thumb/a"),
		"another server's poster at the same path",
	)
	assert.NotEqual(
		t,
		CacheID("ab", "c"),
		CacheID("a", "bc"),
		"the server and path cannot run together",
	)
}

func TestCacheControl(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "private, max-age=604800, immutable", CacheControl,
		"a signed-in user's thumbnail is not kept by a shared cache")
}
