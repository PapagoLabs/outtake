// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// mediaListQuery is the media library browse query state.
type mediaListQuery struct {
	Query     string
	LibraryID string
	ParentID  string
	Sort      string
	Letter    string
	Start     int
	Before    int
}

const (
	// QueryLetter is the media library first-character jump parameter.
	queryLetter = "letter"

	// queryBefore is the exclusive end offset when prepending a previous page.
	queryBefore = "before"

	// addedIndexPageSize is the PMS page size used to build added-at buckets.
	addedIndexPageSize = 200

	// maxJumpLabels is the target number of marks on the jump rail.
	maxJumpLabels = 28

	// MediaSortTitleAsc lists titles A-Z.
	mediaSortTitleAsc = "title_asc"
	// MediaSortTitleDesc lists titles Z-A.
	mediaSortTitleDesc = "title_desc"
	// MediaSortYearDesc lists newest years first.
	mediaSortYearDesc = "year_desc"
	// MediaSortYearAsc lists oldest years first.
	mediaSortYearAsc = "year_asc"
	// MediaSortAddedDesc lists recently added titles first.
	mediaSortAddedDesc = "added_desc"
	// MediaSortAddedAsc lists oldest added titles first.
	mediaSortAddedAsc = "added_asc"
)

// parseMediaListQuery reads search, library, parent, sort, letter, start, and before.
//
// Invalid sort values fall back to title A-Z on a library root.
// Nested containers drop sort and letter.
//
// Parameters:
//   - ctx: Request with optional q, library, parent, sort, letter, start, and before.
//
// Returns:
//   - query: Normalized browse state.
func parseMediaListQuery(ctx fiber.Ctx) mediaListQuery {
	return normalizeMediaListQuery(mediaListQuery{
		Query:     ctx.Query(queryQ),
		LibraryID: ctx.Query(queryLibrary),
		ParentID:  ctx.Query(queryParent),
		Sort:      ctx.Query(querySort),
		Letter:    ctx.Query(queryLetter),
		Start:     pageStart(ctx.Query(queryStart)),
		Before:    pageStart(ctx.Query(queryBefore)),
	})
}

// normalizeMediaListQuery drops unknown sort values and nested letter jumps.
//
// Parameters:
//   - query: Raw browse query from the request.
//
// Returns:
//   - query: Normalized browse state.
func normalizeMediaListQuery(query mediaListQuery) mediaListQuery {
	if query.ParentID != "" {
		query.Sort = ""
		query.Letter = ""
		query.Before = 0

		return query
	}

	switch query.Sort {
	case mediaSortTitleAsc,
		mediaSortTitleDesc,
		mediaSortYearDesc,
		mediaSortYearAsc,
		mediaSortAddedDesc,
		mediaSortAddedAsc:
	default:
		if query.LibraryID != "" {
			query.Sort = mediaSortTitleAsc
		} else {
			query.Sort = ""
		}
	}

	if query.Query != "" {
		query.Letter = ""
		query.Before = 0
	}

	return query
}

// isLibraryRoot reports whether the query is a section listing.
//
// Returns:
//   - ok: True when a library is selected without a parent or search.
func (query mediaListQuery) isLibraryRoot() bool {
	return query.LibraryID != "" && query.ParentID == "" && query.Query == ""
}

// listStart returns the Plex container offset for this query.
//
// A positive start wins over letter so load-more URLs keep their offset.
//
// Parameters:
//   - index: First-character buckets used when letter is set and start is 0.
//
// Returns:
//   - start: Container offset.
func (query mediaListQuery) listStart(index []plex.LetterIndex) int {
	if query.Start > 0 {
		return query.Start
	}

	if query.Letter == "" {
		return 0
	}

	return plex.LetterOffset(index, query.Letter)
}

// window returns the PMS offset and page size for this query.
//
// Parameters:
//   - index: First-character buckets used when letter is set and start is 0.
//
// Returns:
//   - start: Container offset.
//   - size: Page size for this window.
func (query mediaListQuery) window(index []plex.LetterIndex) (int, int) {
	if query.Before > 0 {
		start := query.Before - mediaPageSize
		if start < 0 {
			start = 0
		}

		return start, query.Before - start
	}

	return query.listStart(index), mediaPageSize
}

// showJumpIndex reports whether the library root has a jump rail.
//
// Returns:
//   - ok: True when the query is a library section listing.
func (query mediaListQuery) showJumpIndex() bool {
	return query.isLibraryRoot()
}

// plexMediaSort maps an Outtake sort key onto a PMS sort value.
//
// Parameters:
//   - sort: Normalized Outtake sort key.
//
// Returns:
//   - plexSort: PMS sort query value, or empty when sort is unset.
func plexMediaSort(sort string) string {
	switch sort {
	case mediaSortTitleAsc:
		return "titleSort:asc"
	case mediaSortTitleDesc:
		return "titleSort:desc"
	case mediaSortYearDesc:
		return "year:desc"
	case mediaSortYearAsc:
		return "year:asc"
	case mediaSortAddedDesc:
		return "addedAt:desc"
	case mediaSortAddedAsc:
		return "addedAt:asc"
	default:
		return ""
	}
}

// toLetterIndexes maps PMS first-character buckets onto page models.
//
// Empty buckets are omitted. Start is the cumulative offset of each letter.
//
// Parameters:
//   - index: PMS first-character directories.
//
// Returns:
//   - letters: Non-empty letters with jump offsets.
func toLetterIndexes(index []plex.LetterIndex) []view.LetterIndex {
	letters := make([]view.LetterIndex, 0, len(index))
	start := 0

	for _, entry := range index {
		if entry.Size > 0 {
			letters = append(letters, view.LetterIndex{
				Title: entry.Title,
				Size:  entry.Size,
				Start: start,
			})
		}

		start += entry.Size
	}

	return letters
}

// thinJumpIndexes reduces rail marks to a readable set of nice ticks.
//
// Parameters:
//   - letters: Jump targets for the active sort.
//   - sort: Normalized Outtake sort key.
//
// Returns:
//   - letters: A thinned list of rail marks.
func thinJumpIndexes(letters []view.LetterIndex, sort string) []view.LetterIndex {
	if len(letters) <= maxJumpLabels {
		return letters
	}

	switch sort {
	case mediaSortYearDesc, mediaSortYearAsc:
		return thinNumericTitles(letters, yearTickStep(len(letters)))
	case mediaSortAddedDesc, mediaSortAddedAsc:
		return thinMonthTitles(letters)
	default:
		return letters
	}
}

// yearTickStep chooses the year spacing for a crowded jump rail.
//
// Parameters:
//   - n: Number of year buckets.
//
// Returns:
//   - step: Year modulus used to keep marks.
func yearTickStep(n int) int {
	switch {
	case n <= maxJumpLabels:
		return 1
	case n <= 70:
		return 5
	default:
		return 10
	}
}

// thinNumericTitles keeps first, last, and step-aligned numeric titles.
//
// Parameters:
//   - letters: Numeric jump targets in display order.
//   - step: Year modulus to keep.
//
// Returns:
//   - letters: Thinned numeric marks.
func thinNumericTitles(letters []view.LetterIndex, step int) []view.LetterIndex {
	if step <= 1 || len(letters) <= 2 {
		return letters
	}

	keep := make([]bool, len(letters))
	keep[0] = true
	keep[len(letters)-1] = true

	for i, letter := range letters {
		year, err := strconv.Atoi(letter.Title)
		if err != nil {
			continue
		}

		if year%step == 0 {
			keep[i] = true
		}
	}

	out := make([]view.LetterIndex, 0, maxJumpLabels)
	for i, letter := range letters {
		if keep[i] {
			out = append(out, letter)
		}
	}

	if len(out) > maxJumpLabels {
		return sampleJumpIndexes(out, maxJumpLabels)
	}

	return out
}

// thinMonthTitles keeps one month mark per year, then samples if still long.
//
// Parameters:
//   - letters: Month jump targets in display order.
//
// Returns:
//   - letters: Thinned month marks.
func thinMonthTitles(letters []view.LetterIndex) []view.LetterIndex {
	if len(letters) <= maxJumpLabels {
		return letters
	}

	yearly := make([]view.LetterIndex, 0)
	lastYear := ""

	for _, letter := range letters {
		year := monthJumpYear(letter.Title)
		if year == lastYear && len(yearly) > 0 {
			continue
		}

		yearly = append(yearly, letter)
		lastYear = year
	}

	if len(yearly) <= maxJumpLabels {
		return yearly
	}

	return sampleJumpIndexes(yearly, maxJumpLabels)
}

// monthJumpYear extracts the year from an mm/yyyy jump title.
//
// Parameters:
//   - title: Jump label such as 03/2024.
//
// Returns:
//   - year: Trailing four characters, or the whole title when shorter.
func monthJumpYear(title string) string {
	if len(title) >= 4 {
		return title[len(title)-4:]
	}

	return title
}

// sampleJumpIndexes picks evenly spaced marks including the ends.
//
// Parameters:
//   - letters: Jump targets in display order.
//   - max: Maximum number of marks to keep.
//
// Returns:
//   - letters: Sampled marks.
func sampleJumpIndexes(letters []view.LetterIndex, max int) []view.LetterIndex {
	if len(letters) <= max || max < 2 {
		return letters
	}

	out := make([]view.LetterIndex, 0, max)
	last := len(letters) - 1
	out = append(out, letters[0])

	inner := max - 2
	for i := 1; i <= inner; i++ {
		idx := i * last / (inner + 1)
		if idx <= 0 || idx >= last {
			continue
		}

		if out[len(out)-1].Title == letters[idx].Title {
			continue
		}

		out = append(out, letters[idx])
	}

	if out[len(out)-1].Title != letters[last].Title {
		out = append(out, letters[last])
	}

	return out
}

// orderJumpIndex sorts or reverses buckets to match the active sort.
//
// Parameters:
//   - index: PMS buckets in default order.
//   - sort: Normalized Outtake sort key.
//
// Returns:
//   - index: Buckets ordered for the jump rail.
func orderJumpIndex(index []plex.LetterIndex, sort string) []plex.LetterIndex {
	switch sort {
	case mediaSortTitleDesc:
		return plex.ReverseIndexes(index)
	case mediaSortYearDesc:
		return plex.SortYearIndexes(index, true)
	case mediaSortYearAsc:
		return plex.SortYearIndexes(index, false)
	default:
		return index
	}
}

// addedMonthKey formats a Plex addedAt unix timestamp as mm/yyyy.
//
// Parameters:
//   - addedAt: Unix seconds, or 0 when unknown.
//
// Returns:
//   - key: Month label, or # when addedAt is unset.
func addedMonthKey(addedAt int64) string {
	if addedAt <= 0 {
		return "#"
	}

	return time.Unix(addedAt, 0).Local().Format("01/2006")
}

// addedAtIndexes builds mm/yyyy buckets from a sorted library listing.
//
// Parameters:
//   - items: Media items in added-at order.
//
// Returns:
//   - index: Consecutive month buckets.
func addedAtIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return groupIndexes(items, func(item plex.MediaItem) string {
		return addedMonthKey(item.AddedAt)
	})
}

// yearIndexes builds year buckets from a sorted library listing.
//
// Parameters:
//   - items: Media items in year order.
//
// Returns:
//   - index: Consecutive year buckets.
func yearIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return groupIndexes(items, func(item plex.MediaItem) string {
		if item.Year <= 0 {
			return "#"
		}

		return strconv.Itoa(item.Year)
	})
}

// titleIndexes builds first-character buckets from a sorted library listing.
//
// Parameters:
//   - items: Media items in title order.
//
// Returns:
//   - index: Consecutive letter buckets.
func titleIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return groupIndexes(items, func(item plex.MediaItem) string {
		return titleJumpKey(item.TitleSort, item.Title)
	})
}

// titleJumpKey returns the first-letter jump key for a title.
//
// Parameters:
//   - titleSort: PMS titleSort value, preferred when set.
//   - title: Display title used when titleSort is empty.
//
// Returns:
//   - key: A through Z, or # for anything else.
func titleJumpKey(titleSort, title string) string {
	s := strings.TrimSpace(titleSort)
	if s == "" {
		s = strings.TrimSpace(title)
	}

	if s == "" {
		return "#"
	}

	r := unicode.ToUpper([]rune(s)[0])
	if r >= 'A' && r <= 'Z' {
		return string(r)
	}

	return "#"
}

// groupIndexes collapses consecutive items that share a jump key.
//
// Parameters:
//   - items: Media items in display order.
//   - keyFn: Jump key for each item.
//
// Returns:
//   - index: Consecutive buckets with sizes.
func groupIndexes(items []plex.MediaItem, keyFn func(plex.MediaItem) string) []plex.LetterIndex {
	index := make([]plex.LetterIndex, 0)
	last := ""

	for _, item := range items {
		key := keyFn(item)
		if key == last && len(index) > 0 {
			index[len(index)-1].Size++

			continue
		}

		index = append(index, plex.LetterIndex{Title: key, Size: 1})
		last = key
	}

	return index
}

// collectAddedAtItems pages a sorted library listing for jump-rail grouping.
//
// Parameters:
//   - ctx: Request context.
//   - client: PMS client.
//   - server: PMS to query.
//   - libraryID: Section key.
//   - sort: Normalized Outtake sort key.
//
// Returns:
//   - items: Collected media items, or a prefix when a page fails.
func collectAddedAtItems(
	ctx context.Context,
	client *plex.Client,
	server plex.Server,
	libraryID, sort string,
) []plex.MediaItem {
	items := make([]plex.MediaItem, 0)
	start := 0

	for {
		page, err := client.GetMediaPage(ctx, server, libraryID, start, addedIndexPageSize, plexMediaSort(sort))
		if err != nil {
			return items
		}

		items = append(items, page.Items...)
		start += len(page.Items)
		if len(page.Items) == 0 || start >= page.Total {
			return items
		}
	}
}
