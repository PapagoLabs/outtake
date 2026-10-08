// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package assets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestURLCarriesTheContentHash covers the asset URL: a known file carries a
// sixteen-digit hash that Current accepts, with or without the route prefix.
func TestURLCarriesTheContentHash(t *testing.T) {
	t.Parallel()

	url := URL("css/output.css")

	assert.Regexp(t, `^/assets/css/output\.css\?v=[0-9a-f]{16}$`, url)
	assert.Equal(t, url, URL("/assets/css/output.css"), "the route prefix is optional")

	version := url[len(url)-versionLength:]
	assert.True(t, Current("css/output.css", version))
	assert.True(t, Current("/assets/css/output.css", version))
	assert.False(t, Current("css/output.css", "0000000000000000"), "a stale version is not current")
	assert.False(t, Current("css/output.css", ""), "no version is not current")
	assert.NotEqual(t, url, URL("js/app.js"), "each file has its own hash")
}

// TestURLLeavesAnUnknownFileUnversioned covers a name nothing was embedded
// under, which cannot be named by its contents.
func TestURLLeavesAnUnknownFileUnversioned(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/assets/js/missing.js", URL("js/missing.js"))
	assert.False(t, Current("js/missing.js", "anything"))
}
