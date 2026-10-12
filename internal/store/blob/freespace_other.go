// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !linux && !darwin

package blob

// FreeSpace reports how much room the volume holding a path has.
//
// It is not measured on this system, so the storage page leaves it out.
//
// Returns:
//   - free: Always 0.
//   - total: Always 0.
//   - ok: Always false.
//
//nolint:nonamedreturns // The named results tell the two sizes apart.
func FreeSpace(string) (free, total uint64, ok bool) {
	return 0, 0, false
}
