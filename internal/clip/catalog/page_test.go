// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// numbered returns clips named by their place in a list.
//
// Parameters:
//   - count: How many clips.
//
// Returns:
//   - jobs: The clips, ids 0 to count-1.
func numbered(count int) []*clip.Job {
	jobs := make([]*clip.Job, 0, count)

	for index := range count {
		jobs = append(jobs, stored(strconv.Itoa(index)))
	}

	return jobs
}

// TestWindowPagesThroughAList covers reading a list a page at a time.
//
// Each page holds PageSize clips, the last holds what is left, and every clip
// is read once.
func TestWindowPagesThroughAList(t *testing.T) {
	t.Parallel()

	jobs := numbered(PageSize*2 + 5)
	query := Normalize(ClipListQuery{})
	seen := 0

	for page := Window(jobs, query); ; page = Window(jobs, query) {
		assert.Equal(t, strconv.Itoa(seen), page.Jobs[0].ID)

		seen += len(page.Jobs)

		if !page.More {
			assert.Len(t, page.Jobs, 5)

			break
		}

		assert.Len(t, page.Jobs, PageSize)

		query.After = page.Last()
		query.Offset = page.Next
	}

	assert.Equal(t, len(jobs), seen)
}

// TestWindowFollowsTheLastClipShown covers a clip made while a list is read.
//
// Under a sort by creation, the next page still starts after the last card
// shown, so no card repeats. Under a sort by modification, or when that clip
// is gone, the page starts pageOverlap clips before the offset.
func TestWindowFollowsTheLastClipShown(t *testing.T) {
	t.Parallel()

	jobs := numbered(PageSize + 3)
	first := Window(jobs, Normalize(ClipListQuery{}))
	grown := append([]*clip.Job{stored("new")}, jobs...)

	next := Window(grown, Normalize(ClipListQuery{After: first.Last(), Offset: first.Next}))
	assert.Equal(t, strconv.Itoa(PageSize), next.Jobs[0].ID, "the new clip moves no card")

	modified := Window(grown, Normalize(ClipListQuery{
		Sort: SortUpdatedDesc, After: first.Last(), Offset: first.Next,
	}))
	assert.Equal(t, strconv.Itoa(PageSize-1-pageOverlap), modified.Jobs[0].ID)

	gone := Window(jobs, Normalize(ClipListQuery{After: "absent", Offset: pageOverlap + 2}))
	assert.Equal(t, "2", gone.Jobs[0].ID)
}

// TestWindowOverlapKeepsAMovedClipFromHidingAnother covers a clip shown on
// the first page that moves to the end of a list sorted by modification.
//
// The rest shift up one place, and the next page still holds the clip that
// took the offset's place.
func TestWindowOverlapKeepsAMovedClipFromHidingAnother(t *testing.T) {
	t.Parallel()

	jobs := numbered(PageSize + 10)
	query := Normalize(ClipListQuery{Sort: SortUpdatedAsc})
	first := Window(jobs, query)
	moved := append(append([]*clip.Job{}, jobs[1:]...), jobs[0])

	query.After = first.Last()
	query.Offset = first.Next

	next := Window(moved, query)

	assert.Equal(t, strconv.Itoa(PageSize), next.Jobs[pageOverlap-1].ID,
		"the clip that moved into the offset's place is shown")
	assert.Equal(t, PageSize+10, next.Next)
}

// TestWindowPastTheEndIsEmpty covers an offset past the list and an empty
// list.
func TestWindowPastTheEndIsEmpty(t *testing.T) {
	t.Parallel()

	past := Window(numbered(3), Normalize(ClipListQuery{Offset: 10}))
	assert.Empty(t, past.Jobs)
	assert.False(t, past.More)
	assert.Empty(t, past.Last())

	assert.Empty(t, Window(nil, Normalize(ClipListQuery{Offset: -4})).Jobs,
		"a negative offset reads from the start")
}
