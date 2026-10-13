// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBulkMessageSaysWhatHappened covers the status line for each mix of
// counts.
//
// One sentence ends without a period, several each end with one, and every
// count agrees with its number.
func TestBulkMessageSaysWhatHappened(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		action bulkAction
		result bulkResult
		limit  string
		want   string
	}{
		"one done": {
			action: bulkDelete,
			result: bulkResult{done: 1},
			want:   "Deleted 1 clip",
		},
		"several done": {
			action: bulkUnshare,
			result: bulkResult{done: 3},
			want:   "Stopped sharing 3 clips",
		},
		"done and skipped": {
			action: bulkCancel,
			result: bulkResult{done: 2, skipped: 1},
			want:   "Canceled 2 clips. 1 clip was skipped.",
		},
		"failed": {
			action: bulkShare,
			result: bulkResult{failed: 2},
			want:   "2 clips couldn't be changed. Check the Outtake log.",
		},
		"stopped at a limit": {
			action: bulkRegenerate,
			result: bulkResult{done: 1, notStarted: 1},
			limit:  "You've reached your limit of 1 concurrent render. Wait for one to finish.",
			want: "Regenerated 1 clip. 1 clip wasn't started. " +
				"You've reached your limit of 1 concurrent render. Wait for one to finish.",
		},
		"nothing": {
			action: bulkRegenerate,
			result: bulkResult{},
			want:   msgNothingChanged,
		},
	} {
		assert.Equal(t, test.want, bulkMessage(test.action, test.result, test.limit), name)
	}
}

// TestDistinctIDsKeepsTheFirstOfEach covers the clips a request names.
func TestDistinctIDsKeepsTheFirstOfEach(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"b", "a"}, distinctIDs([]string{"b", "", "a", "b"}))
}
