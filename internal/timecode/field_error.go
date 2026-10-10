// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package timecode

import (
	"fmt"
)

// FieldError reports a form field that holds something other than a
// timecode. It names the field, so the page can say which one to fix.
type FieldError struct {
	// Field names the field, such as "start" or "end".
	Field string
	// Value is what the field held.
	Value string
}

// Error describes the field and what it held.
//
// Returns:
//   - message: The error text.
func (err *FieldError) Error() string {
	return fmt.Sprintf(
		"%v: the %s must be a timecode such as 00:01:23.456, not %q",
		ErrInvalidTimecode,
		err.Field,
		err.Value,
	)
}

// Unwrap ties the error to ErrInvalidTimecode.
//
// Returns:
//   - target: ErrInvalidTimecode.
func (*FieldError) Unwrap() error {
	return ErrInvalidTimecode
}
