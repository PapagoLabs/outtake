// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"context"
)

// Failure is an error as a page shows it: a plain message, and the details
// someone debugging it or reporting an issue needs, which the page keeps in a
// collapsed Details section with a copy button.
type Failure struct {
	// Message says what went wrong in plain words.
	Message string `json:"message"`
	// Details is the technical report: the version, time, request, reference,
	// and error chain. It is empty when the message says all there is.
	Details string `json:"details"`
}

// failureKey is the context key for the failure a page renders.
type failureKey struct{}

// NewNotice is a failure with a message and no details, for a refusal the
// message explains in full, such as a field that needs a value.
//
// Parameters:
//   - message: The plain message.
//
// Returns:
//   - failure: The failure, empty when message is.
func NewNotice(message string) Failure {
	return Failure{Message: message, Details: ""}
}

// ContextWithFailure stores the failure a page renders in its banner slot.
//
// Parameters:
//   - parent: Parent context.
//   - failure: The failure to show.
//
// Returns:
//   - ctx: Derived context that carries the failure.
func ContextWithFailure(parent context.Context, failure Failure) context.Context {
	return context.WithValue(parent, failureKey{}, failure)
}

// FailureFromContext returns the failure a page renders in its banner slot.
//
// Parameters:
//   - ctx: Render context, optionally carrying a failure.
//
// Returns:
//   - failure: The stored failure, or an empty one.
func FailureFromContext(ctx context.Context) Failure {
	failure, ok := ctx.Value(failureKey{}).(Failure)
	if !ok {
		return Failure{Message: "", Details: ""}
	}

	return failure
}

// Empty reports whether there is nothing to show.
//
// Returns:
//   - empty: True when the failure has no message.
func (failure Failure) Empty() bool {
	return failure.Message == ""
}
