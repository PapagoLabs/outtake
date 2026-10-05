// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"slices"
	"strings"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// ClipListQuery is the normalized clips toolbar state, which says which clips are listed and in what order.
type ClipListQuery struct {
	Status clip.Status
	Type   clip.Type
	Query  string
	Sort   string
}

const (
	// SortCreatedDesc lists newest created clips first.
	SortCreatedDesc = "created_desc"
	// SortCreatedAsc lists oldest created clips first.
	SortCreatedAsc = "created_asc"
	// SortUpdatedDesc lists recently modified clips first.
	SortUpdatedDesc = "updated_desc"
	// SortUpdatedAsc lists oldest modified clips first.
	SortUpdatedAsc = "updated_asc"
	// SortNameAsc lists clips by display name A-Z.
	SortNameAsc = "name_asc"
	// SortNameDesc lists clips by display name Z-A.
	SortNameDesc = "name_desc"
)

// Normalize drops unknown type and sort values.
//
// Parameters:
//   - query: Raw toolbar state.
//
// Returns:
//   - query: State carrying only the filters and sort keys declared here.
func Normalize(query ClipListQuery) ClipListQuery {
	if _, ok := clip.ParseType(string(query.Type)); !ok {
		query.Type = ""
	}

	if _, ok := clip.ParseStatus(string(query.Status)); !ok {
		query.Status = ""
	}

	switch query.Sort {
	case SortCreatedAsc, SortUpdatedDesc, SortUpdatedAsc, SortNameAsc, SortNameDesc:
	default:
		query.Sort = SortCreatedDesc
	}

	return query
}

// Apply filters then sorts jobs for the clips toolbar.
//
// Parameters:
//   - jobs: Unfiltered renders.
//   - query: Normalized filters and sort.
//
// Returns:
//   - jobs: Matching renders in the requested order.
func Apply(jobs []*clip.Job, query ClipListQuery) []*clip.Job {
	needle := strings.ToLower(query.Query)
	matched := make([]*clip.Job, 0, len(jobs))

	for _, job := range jobs {
		if !matches(job, query, needle) {
			continue
		}

		matched = append(matched, job)
	}

	slices.SortFunc(matched, func(left, right *clip.Job) int {
		return compare(&left.Clip, &right.Clip, query.Sort)
	})

	return matched
}
