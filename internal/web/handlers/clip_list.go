// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"cmp"
	"slices"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
)

// clipListQuery is the Clips page and media-item clip-list toolbar state.
type clipListQuery struct {
	Status string
	Type   string
	Query  string
	Sort   string
}

const (
	// QueryType is the clip type filter query parameter.
	queryType = "type"
	// QueryQ is the clip name search query parameter.
	queryQ = "q"
	// QuerySort is the clip list sort query parameter.
	querySort = "sort"

	// ClipSortCreatedDesc lists newest created clips first.
	clipSortCreatedDesc = "created_desc"
	// ClipSortCreatedAsc lists oldest created clips first.
	clipSortCreatedAsc = "created_asc"
	// ClipSortUpdatedDesc lists recently modified clips first.
	clipSortUpdatedDesc = "updated_desc"
	// ClipSortUpdatedAsc lists oldest modified clips first.
	clipSortUpdatedAsc = "updated_asc"
	// ClipSortNameAsc lists clips by display name A-Z.
	clipSortNameAsc = "name_asc"
	// ClipSortNameDesc lists clips by display name Z-A.
	clipSortNameDesc = "name_desc"

	// ClipTypeClip is the video clip type filter.
	clipTypeClip = "clip"
	// ClipTypeGIF is the GIF type filter.
	clipTypeGIF = "gif"
	// ClipTypeScreenshot is the screenshot type filter.
	clipTypeScreenshot = "screenshot"
)

// parseClipListQuery reads type, name, sort, and status from the request.
//
// Invalid type or sort values are ignored and replaced with defaults.
//
// Parameters:
//   - ctx: Request with optional type, q, sort, and status query parameters.
//
// Returns:
//   - query: Normalized list filters and sort.
func parseClipListQuery(ctx fiber.Ctx) clipListQuery {
	return normalizeClipListQuery(clipListQuery{
		Status: ctx.Query("status"),
		Type:   ctx.Query(queryType),
		Query:  strings.TrimSpace(ctx.Query(queryQ)),
		Sort:   ctx.Query(querySort),
	})
}

// normalizeClipListQuery drops unknown type and sort values.
func normalizeClipListQuery(query clipListQuery) clipListQuery {
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

// filtered reports whether any list filter is active.
func (query clipListQuery) filtered() bool {
	return query.Status != "" || query.Type != "" || query.Query != ""
}

// applyClipListQuery filters then sorts jobs for the clips toolbar.
//
// Parameters:
//   - jobs: Unfiltered clip jobs.
//   - query: Normalized filters and sort.
//
// Returns:
//   - filtered: Matching jobs in the requested order.
func applyClipListQuery(jobs []*queue.Job, query clipListQuery) []*queue.Job {
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
func clipJobMatches(job *queue.Job, query clipListQuery, needle string) bool {
	if query.Status != "" && !clipMatchesStatus(string(job.Status), query.Status) {
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

// compareClipListJobs orders two jobs by the requested sort, then id descending.
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
func jobDisplayName(job *queue.Job) string {
	if job.Name != "" {
		return job.Name
	}

	return job.MediaTitle
}

// clipNameKey is the case-insensitive display name used for name sorts.
func clipNameKey(job *queue.Job) string {
	return strings.ToLower(jobDisplayName(job))
}
