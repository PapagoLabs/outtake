// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

// Status is how far a render has got.
type Status string

const (
	// StatusPending is a render that has not started.
	StatusPending Status = "pending"

	// StatusProcessing is a render that is running.
	StatusProcessing Status = "processing"

	// StatusCompleted is a render that finished.
	StatusCompleted Status = "completed"

	// StatusFailed is a render that stopped on an error.
	StatusFailed Status = "failed"

	// StatusCancelled is a render the user stopped.
	StatusCancelled Status = "canceled"
)

// ParseStatus resolves a requested render status.
//
// Parameters:
//   - raw: Requested status.
//
// Returns:
//   - The canonical status.
//   - False when the status is not one this app produces.
func ParseStatus(raw string) (Status, bool) {
	switch state := Status(raw); state {
	case StatusPending, StatusProcessing, StatusCompleted, StatusFailed, StatusCancelled:
		return state, true
	default:
		return "", false
	}
}
