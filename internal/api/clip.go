// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package api names the wire types handlers still import. The definitions live
// with the noun they describe.
package api

import (
	"github.com/PapagoLabs/outtake/internal/clip"
)

// ClipRequest is the export request. The value is a clip.Request.
type ClipRequest = clip.Request

// ClipResponse is the clip payload. The value is a clip.Response.
type ClipResponse = clip.Response

// Flag reads an optional request flag, treating an absent one as false.
//
// Parameters:
//   - value: Submitted flag, nil when the request omitted the field.
//
// Returns:
//   - flag: The value the flag carries, false when it was absent.
func Flag(value *bool) bool {
	return clip.Flag(value)
}

// FlagOrDefault reads an optional request flag against a configured default.
//
// Parameters:
//   - value: Submitted flag, nil when the request omitted the field.
//   - fallback: Configured value to use when the field was absent.
//
// Returns:
//   - flag: The value the flag carries, otherwise fallback.
func FlagOrDefault(value *bool, fallback bool) bool {
	return clip.FlagOrDefault(value, fallback)
}
