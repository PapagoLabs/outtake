// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"cmp"
	"slices"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	sharedplex "github.com/PapagoLabs/outtake/internal/web/handlers/shared/plex"
)

// ListQuery is the Clips page and media-item clip-list toolbar state.
type ListQuery struct {
	Status string
	Type   string
	Query  string
	Sort   string
}

const (
	// queryType is the clip type filter query parameter.
	queryType = "type"
	// queryQ is the clip name search query parameter.
	queryQ = "q"
	// querySort is the clip list sort query parameter.
	querySort = "sort"

	// clipSortCreatedDesc lists newest created clips first.
	clipSortCreatedDesc = "created_desc"
	// clipSortCreatedAsc lists oldest created clips first.
	clipSortCreatedAsc = "created_asc"
	// clipSortUpdatedDesc lists recently modified clips first.
	clipSortUpdatedDesc = "updated_desc"
	// clipSortUpdatedAsc lists oldest modified clips first.
	clipSortUpdatedAsc = "updated_asc"
	// clipSortNameAsc lists clips by display name A-Z.
	clipSortNameAsc = "name_asc"
	// clipSortNameDesc lists clips by display name Z-A.
	clipSortNameDesc = "name_desc"

	// clipTypeClip is the video clip type filter.
	clipTypeClip = "clip"
	// clipTypeGIF is the GIF type filter.
	clipTypeGIF = "gif"
	// clipTypeScreenshot is the screenshot type filter.
	clipTypeScreenshot = "screenshot"
)

// ParseListQuery reads type, name, sort, and status from the request.
//
// Invalid type or sort values are ignored and replaced with defaults.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - query: Parsed clip list toolbar state.
func ParseListQuery(ctx fiber.Ctx) ListQuery {
	return NormalizeListQuery(ListQuery{
		Status: ctx.Query("status"),
		Type:   ctx.Query(queryType),
		Query:  strings.TrimSpace(ctx.Query(queryQ)),
		Sort:   ctx.Query(querySort),
	})
}

// NormalizeListQuery drops unknown type and sort values.
//
// Parameters:
//   - query: Raw clip list query values.
//
// Returns:
//   - normalized: Query with invalid type or sort cleared.
func NormalizeListQuery(query ListQuery) ListQuery {
	switch query.Type {
	case clipTypeClip, clipTypeGIF, clipTypeScreenshot:
	default:
		query.Type = ""
	}

	switch query.Sort {
	case clipSortCreatedAsc,
		clipSortUpdatedDesc,
		clipSortUpdatedAsc,
		clipSortNameAsc,
		clipSortNameDesc:
	default:
		query.Sort = clipSortCreatedDesc
	}

	return query
}

// Filtered reports whether any list filter is active.
//
// Returns:
//   - ok: True when any list filter is active.
func (query ListQuery) Filtered() bool {
	return query.Status != "" || query.Type != "" || query.Query != ""
}

// ApplyListQuery filters then sorts jobs for the clips toolbar.
//
// Parameters:
//   - jobs: Clip jobs to filter and sort.
//   - query: Active clips toolbar filters.
//
// Returns:
//   - filtered: Matching jobs in sort order.
func ApplyListQuery(jobs []*queue.Job, query ListQuery) []*queue.Job {
	needle := strings.ToLower(query.Query)
	filtered := make([]*queue.Job, 0, len(jobs))

	for _, job := range jobs {
		if !clipJobMatches(job, query, needle) {
			continue
		}

		filtered = append(filtered, job)
	}

	slices.SortFunc(filtered, func(left, right *queue.Job) int {
		return compareClipListJobs(left, right, query.Sort)
	})

	return filtered
}

// clipJobMatches reports whether a job passes status, type, and name filters.
//
// Parameters:
//   - job: Clip job to process or persist.
//   - query: Search or filter query string.
//   - needle: Substring to match against clip fields.
//
// Returns:
//   - ok: True when a job passes status, type, and name filters.
func clipJobMatches(job *queue.Job, query ListQuery, needle string) bool {
	if query.Status != "" && !sharedplex.ClipMatchesStatus(string(job.Status), query.Status) {
		return false
	}

	if query.Type != "" && string(job.Type) != query.Type {
		return false
	}

	if needle == "" {
		return true
	}

	return strings.Contains(strings.ToLower(job.Name), needle) ||
		strings.Contains(strings.ToLower(job.MediaTitle), needle)
}

// compareClipListJobs orders two jobs by the requested sort, then id
// descending.
//
// Parameters:
//   - left: Left operand for comparison.
//   - right: Right operand for comparison.
//   - sort: Typed string argument for compareClipListJobs.
//
// Returns:
//   - n: Numeric result for this call.
func compareClipListJobs(left, right *queue.Job, sort string) int {
	var order int

	switch sort {
	case clipSortCreatedAsc:
		order = left.CreatedAt.Compare(right.CreatedAt)
	case clipSortUpdatedDesc:
		order = right.UpdatedAt.Compare(left.UpdatedAt)
	case clipSortUpdatedAsc:
		order = left.UpdatedAt.Compare(right.UpdatedAt)
	case clipSortNameAsc:
		order = cmp.Compare(clipNameKey(left), clipNameKey(right))
	case clipSortNameDesc:
		order = cmp.Compare(clipNameKey(right), clipNameKey(left))
	default:
		order = right.CreatedAt.Compare(left.CreatedAt)
	}

	if order != 0 {
		return order
	}

	return cmp.Compare(right.ID, left.ID)
}

// jobDisplayName is the clip name, or the media title when the name is empty.
//
// Parameters:
//   - job: Clip job to process or persist.
//
// Returns:
//   - value: Result value; zero or empty when unavailable.
func jobDisplayName(job *queue.Job) string {
	if job.Name != "" {
		return job.Name
	}

	return job.MediaTitle
}

// clipNameKey is the case-insensitive display name used for name sorts.
//
// Parameters:
//   - job: Clip job to process or persist.
//
// Returns:
//   - value: Result value; zero or empty when unavailable.
func clipNameKey(job *queue.Job) string {
	return strings.ToLower(jobDisplayName(job))
}
