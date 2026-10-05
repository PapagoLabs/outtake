// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"os"
)

// FileExists reports whether a library artifact is present on disk.
//
// Parameters:
//   - path: Path of the artifact.
//
// Returns:
//   - ok: True when the path names an existing file rather than a directory.
func FileExists(path string) bool {
	if path == "" {
		return false
	}

	info, err := os.Stat(path)

	return err == nil && !info.IsDir()
}
