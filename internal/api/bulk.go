// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

// BulkRequest applies one action to several clips.
type BulkRequest struct {
	// Action is delete, regenerate, cancel, share, or unshare.
	Action string `json:"action"`
	// Clips are the ids of the clips the action applies to.
	Clips []string `json:"clips"`
}

// BulkResponse reports what a bulk action did.
type BulkResponse struct {
	// Action is the action that was applied.
	Action string `json:"action"`
	// Message says what happened, as the page shows it.
	Message string `json:"message"`
	// Done counts the clips the action changed.
	Done int `json:"done"`
	// Skipped counts the clips the action did not apply to, such as a clip the
	// caller may not change or one that is not rendering for a cancel.
	Skipped int `json:"skipped"`
	// Failed counts the clips the action could not change.
	Failed int `json:"failed"`
	// NotStarted counts the clips a reached render limit left alone.
	NotStarted int `json:"notStarted"`
}
