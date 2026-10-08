// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"strings"
	"sync"
)

const (
	// Prefix is the route the assets are served under.
	Prefix = "/assets/"

	// VersionQuery names the query parameter carrying an asset's hash.
	VersionQuery = "v"

	// versionLength is how many hex digits of the hash an asset URL carries.
	versionLength = 16
)

// FS holds the static files. Paths are relative to this directory, such as
// "css/output.css".
//
//go:embed css js brand
var FS embed.FS

// versions maps each embedded file to the hash its URL carries. The files are
// fixed at build time, so they are hashed once. A file that cannot be read is
// left out, so its URL carries no hash and is never cached for good. A walk
// that fails leaves every file out.
var versions = sync.OnceValue(func() map[string]string {
	hashes := map[string]string{}

	err := fs.WalkDir(FS, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		data, readErr := fs.ReadFile(FS, name)
		if readErr != nil {
			return nil
		}

		sum := sha256.Sum256(data)

		hashes[name] = hex.EncodeToString(sum[:])[:versionLength]

		return nil
	})
	if err != nil {
		return map[string]string{}
	}

	return hashes
})

// URL returns the path an asset is served at, carrying the hash of its
// contents. A name nothing was embedded under is returned without a hash.
//
// Parameters:
//   - name: The asset's path inside FS, with or without the Prefix.
//
// Returns:
//   - url: Such as "/assets/css/output.css?v=0123456789abcdef".
func URL(name string) string {
	name = strings.TrimPrefix(name, Prefix)

	version, ok := versions()[name]
	if !ok {
		return Prefix + name
	}

	return Prefix + name + "?" + VersionQuery + "=" + version
}

// Current reports whether a requested version is the hash of the asset as it
// is now, so the response may be cached for good.
//
// Parameters:
//   - name: The asset's path inside FS, with or without the Prefix.
//   - version: The version the request carried, empty for none.
//
// Returns:
//   - current: True when version names the asset's current contents.
func Current(name, version string) bool {
	if version == "" {
		return false
	}

	return versions()[strings.TrimPrefix(name, Prefix)] == version
}
