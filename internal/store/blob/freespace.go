// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build linux || darwin

package blob

import (
	"syscall"
)

// FreeSpace reports how much room the volume holding a path has.
//
// Parameters:
//   - path: Any path on the volume.
//
// Returns:
//   - free: Bytes this process may still write.
//   - total: The volume's size in bytes.
//   - ok: False when the volume cannot be read.
//
//nolint:nonamedreturns // The named results tell the two sizes apart.
func FreeSpace(path string) (free, total uint64, ok bool) {
	var stat syscall.Statfs_t

	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, 0, false
	}

	//nolint:gosec // A block size is positive on every system this builds for.
	blockSize := uint64(stat.Bsize)

	return stat.Bavail * blockSize, stat.Blocks * blockSize, true
}
