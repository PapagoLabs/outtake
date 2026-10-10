// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"errors"
)

// Failure is a failed render as its card shows it: a plain message, and the
// details someone debugging it or reporting an issue needs.
type Failure struct {
	// Message says what went wrong in plain words.
	Message string
	// Details is the technical report: the version, time, reference, subject,
	// and error chain. It is empty when the message says all there is.
	Details string
	// Ref ties the failure to the log line written for it, empty without
	// details.
	Ref string
	// Chain is the error chain as the log writes it, with any secret the
	// describer knows to remove already removed.
	Chain string
}

// DescribeFunc turns the error a render returned into the failure its card
// shows.
//
// Parameters:
//   - subject: What failed, such as the clip id and type, for the details.
//   - err: What the render returned.
//
// Returns:
//   - failure: The message, and the details when they help.
type DescribeFunc func(subject string, err error) Failure

// ErrSourceUnreadable reports a render whose source file cannot be read.
var ErrSourceUnreadable = errors.New("source file is not readable")

// PlainFailure describes a failure by the error's own text, for a service
// that was given no other way to describe one.
//
// Parameters:
//   - _: What failed, which the plain text does not name.
//   - err: What the render returned.
//
// Returns:
//   - failure: The error's text as the message, with no details.
func PlainFailure(_ string, err error) Failure {
	return Failure{Message: err.Error(), Details: "", Ref: "", Chain: err.Error()}
}
