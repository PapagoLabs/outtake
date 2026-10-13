// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"slices"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// ClipPage is one stretch of a sorted clip list.
type ClipPage struct {
	// Jobs are the page's clips, in list order.
	Jobs []*clip.Job
	// Next is how many clips of the list come before the next page.
	Next int
	// More reports that clips follow the page.
	More bool
}

// Page sizes.
const (
	// PageSize is how many clips a list shows before it loads more.
	PageSize = 24
	// pageOverlap is how many clips a page read from its offset repeats from
	// the page before it.
	//
	// A clip shown above the offset that moves down the list or is deleted
	// shifts the rest up, so the overlap keeps a card from being skipped. The
	// page script drops a card that arrives twice.
	pageOverlap = 4
)

// Window picks the page of a sorted list that a query names.
//
// Under a sort by creation, the page starts after the clip query.After names
// while the list still holds it, so a clip made or deleted above it moves no
// card onto another page. A rename moves a clip under a sort by name, and a
// render under a sort by modification, so a page under either sort, or one
// whose clip is gone, starts pageOverlap clips before query.Offset.
//
// Parameters:
//   - jobs: The filtered, sorted list.
//   - query: Normalized toolbar state naming the page.
//
// Returns:
//   - page: Up to PageSize clips and where the next page starts.
func Window(jobs []*clip.Job, query ClipListQuery) ClipPage {
	start := min(max(query.Offset-pageOverlap, 0), len(jobs))

	if query.After != "" && keepsEveryPlace(query.Sort) {
		index := slices.IndexFunc(jobs, func(job *clip.Job) bool { return job.ID == query.After })
		if index >= 0 {
			start = index + 1
		}
	}

	end := min(start+PageSize, len(jobs))

	return ClipPage{Jobs: jobs[start:end], Next: end, More: end < len(jobs)}
}

// Last returns the id of the page's last clip.
//
// Returns:
//   - id: The clip the next page follows, empty for an empty page.
func (page ClipPage) Last() string {
	if len(page.Jobs) == 0 {
		return ""
	}

	return page.Jobs[len(page.Jobs)-1].ID
}

// keepsEveryPlace reports whether a sort never moves a clip it lists.
//
// Parameters:
//   - sort: A normalized sort key.
//
// Returns:
//   - keeps: True for the sorts by when a clip was made, which never changes.
func keepsEveryPlace(sort string) bool {
	return sort == SortCreatedDesc || sort == SortCreatedAsc
}
