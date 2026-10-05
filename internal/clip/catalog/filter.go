// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"cmp"
	"strings"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// Filtered reports whether any list filter is active.
//
// Parameters:
//   - query: Toolbar state.
//
// Returns:
//   - filtered: True when a status, type, or name filter is set.
func (query ClipListQuery) Filtered() bool {
	return query.Status != "" || query.Type != "" || query.Query != ""
}

// MatchesStatus reports whether a job belongs to a status filter.
//
// Parameters:
//   - itemStatus: The job's render status.
//   - want: The requested status filter.
//
// Returns:
//   - matches: True when the job belongs in the filtered list.
func MatchesStatus(itemStatus, want clip.Status) bool {
	switch want {
	case clip.StatusPending:
		return itemStatus == clip.StatusPending || itemStatus == clip.StatusProcessing
	default:
		return itemStatus == want
	}
}

// matches reports whether a job passes the status, type, and name filters.
//
// Parameters:
//   - job: The render to test.
//   - query: Normalized toolbar state.
//   - needle: The lower-cased name search term, empty when the search is off.
//
// Returns:
//   - passes: True when every active filter accepts the job.
func matches(job *clip.Job, query ClipListQuery, needle string) bool {
	if query.Status != "" && !MatchesStatus(job.Status, query.Status) {
		return false
	}

	if query.Type != "" && job.Type != query.Type {
		return false
	}

	if needle == "" {
		return true
	}

	return strings.Contains(strings.ToLower(job.Name), needle) ||
		strings.Contains(strings.ToLower(job.MediaTitle), needle)
}

// compare orders two clip records by the requested sort, then id descending.
// It does not read render state.
//
// Parameters:
//   - left: The first record to order.
//   - right: The second record to order.
//   - sort: The requested sort key.
//
// Returns:
//   - order: Negative when left sorts first, positive when right does.
func compare(left, right *clip.Clip, sort string) int {
	var order int

	switch sort {
	case SortCreatedAsc:
		order = left.CreatedAt.Compare(right.CreatedAt)
	case SortUpdatedDesc:
		order = right.UpdatedAt.Compare(left.UpdatedAt)
	case SortUpdatedAsc:
		order = left.UpdatedAt.Compare(right.UpdatedAt)
	case SortNameAsc:
		order = cmp.Compare(nameKey(left), nameKey(right))
	case SortNameDesc:
		order = cmp.Compare(nameKey(right), nameKey(left))
	default:
		order = right.CreatedAt.Compare(left.CreatedAt)
	}

	if order != 0 {
		return order
	}

	return cmp.Compare(right.ID, left.ID)
}

// nameKey is the case-insensitive display name used for name sorts.
//
// Parameters:
//   - item: Record to read a display name from.
//
// Returns:
//   - name: The display name in lower case.
func nameKey(item *clip.Clip) string {
	return strings.ToLower(clip.DisplayName(item.Name, item.MediaTitle))
}
