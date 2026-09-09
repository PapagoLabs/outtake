// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package media provides FFmpeg-based media processing capabilities.
//
// Crop, progress, timecode, probe, websafe, and quality helpers live in nested
// packages. Package media re-exports their types and functions so importers
// and exec.go keep using this path.
package media
