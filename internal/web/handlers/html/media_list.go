// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package html

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared"
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

// mediaListWindow is the PMS container offset and page size for a query.
type mediaListWindow struct {
	Start int
	Size  int
}

// addedAtIndexCache stores added-at jump buckets by server, library, and sort.
type addedAtIndexCache struct {
	mu      sync.Mutex
	indexes map[string][]plex.LetterIndex
}

const (
	// QueryLetter is the media library first-character jump parameter.
	queryQ = "q"

	// QuerySort is the media library sort parameter.
	querySort = "sort"

	// QueryLetter is the media library first-character jump parameter.
	queryLetter = "letter"

	// QueryBefore is the exclusive end offset when prepending a previous page.
	queryBefore = "before"

	// AddedIndexPageSize is the PMS page size used to build added-at buckets.
	addedIndexPageSize = 200
	// MaxAddedIndexPages caps added-at jump-rail collection.
	maxAddedIndexPages = 10

	// MaxJumpLabels is the target number of marks on the jump rail.
	maxJumpLabels = 28

	// YearTickDenseMax is the year-count cutoff for five-year ticks.
	yearTickDenseMax = 70
	// YearTickDenseStep keeps every fifth year on a moderately long rail.
	yearTickDenseStep = 5
	// YearTickSparseStep keeps every tenth year on a long rail.
	yearTickSparseStep = 10
	// MonthYearLen is the trailing year length in mm/yyyy labels.
	monthYearLen = 4
	// JumpSampleEnds is the first and last marks always kept when sampling.
	jumpSampleEnds = 2
	// JumpOtherKey is the catch-all jump title for unknown letters, years, or dates.
	jumpOtherKey = "#"

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
		LibraryID: ctx.Query(shared.QueryLibrary),
		ParentID:  ctx.Query(shared.QueryParent),
		Sort:      ctx.Query(querySort),
		Letter:    ctx.Query(queryLetter),
		Start:     pageStart(ctx.Query(shared.QueryStart)),
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

// showJumpIndex reports whether the library root has a jump rail.
//
// Returns:
//   - ok: True when the query is a library section listing.
func (query mediaListQuery) showJumpIndex() bool {
	return query.isLibraryRoot()
}

// window returns the PMS offset and page size for this query.
//
// Parameters:
//   - index: First-character buckets used when letter is set and start is 0.
//
// Returns:
//   - window: Container offset and page size.
func (query mediaListQuery) window(index []plex.LetterIndex) mediaListWindow {
	if query.Before > 0 {
		start := max(query.Before-shared.MediaPageSize, 0)

		return mediaListWindow{Start: start, Size: query.Before - start}
	}

	return mediaListWindow{Start: query.listStart(index), Size: shared.MediaPageSize}
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
	case n <= yearTickDenseMax:
		return yearTickDenseStep
	default:
		return yearTickSparseStep
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
	if step <= 1 || len(letters) <= jumpSampleEnds {
		return letters
	}

	out := make([]view.LetterIndex, 0, maxJumpLabels)
	last := len(letters) - 1

	for i, letter := range letters {
		if keepNumericTitle(i, last, letter.Title, step) {
			out = append(out, letter)
		}
	}

	if len(out) > maxJumpLabels {
		return sampleJumpIndexes(out, maxJumpLabels)
	}

	return out
}

// keepNumericTitle reports whether a numeric jump mark should stay on the rail.
//
// Parameters:
//   - index: Position in the ordered year list.
//   - last: Last index in the list.
//   - title: Year label.
//   - step: Year modulus to keep.
//
// Returns:
//   - ok: True for the ends or a step-aligned year.
func keepNumericTitle(index, last int, title string, step int) bool {
	if index == 0 || index == last {
		return true
	}

	year, err := strconv.Atoi(title)
	if err != nil {
		return false
	}

	return year%step == 0
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
	if len(title) >= monthYearLen {
		return title[len(title)-monthYearLen:]
	}

	return title
}

// sampleJumpIndexes picks evenly spaced marks including the ends.
//
// Parameters:
//   - letters: Jump targets in display order.
//   - limit: Maximum number of marks to keep.
//
// Returns:
//   - letters: Sampled marks.
func sampleJumpIndexes(letters []view.LetterIndex, limit int) []view.LetterIndex {
	if len(letters) <= limit || limit < jumpSampleEnds {
		return letters
	}

	last := len(letters) - 1
	out := []view.LetterIndex{letters[0]}

	out = appendInnerJumpMarks(out, letters, limit-jumpSampleEnds, last)

	if out[len(out)-1].Title != letters[last].Title {
		out = append(out, letters[last])
	}

	return out
}

// appendInnerJumpMarks adds evenly spaced interior marks between the ends.
//
// Parameters:
//   - out: Marks already kept, starting with the first letter.
//   - letters: Jump targets in display order.
//   - inner: Number of interior marks to attempt.
//   - last: Last index in letters.
//
// Returns:
//   - out: Marks with interior samples appended.
func appendInnerJumpMarks(
	out, letters []view.LetterIndex,
	inner, last int,
) []view.LetterIndex {
	for i := 1; i <= inner; i++ {
		idx := i * last / (inner + 1)
		if idx <= 0 || idx >= last || out[len(out)-1].Title == letters[idx].Title {
			continue
		}

		out = append(out, letters[idx])
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
		return plex.ReverseIndexes(plex.SortYearIndexes(index))
	case mediaSortYearAsc:
		return plex.SortYearIndexes(index)
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
		return jumpOtherKey
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

// loadAddedAtIndexes returns cached added-at buckets, collecting them on a miss.
//
// Parameters:
//   - ctx: Request context.
//   - cache: Per-handler added-at jump cache.
//   - client: PMS client.
//   - server: PMS to query.
//   - libraryID: Section key.
//   - sort: Normalized Outtake sort key.
//
// Returns:
//   - index: Month buckets for the jump rail.
func loadAddedAtIndexes(
	ctx context.Context,
	cache *addedAtIndexCache,
	client *plex.Client,
	server plex.Server,
	libraryID, sort string,
) []plex.LetterIndex {
	key := addedAtCacheKey(server, libraryID, sort)
	if index, ok := cache.get(key); ok {
		return index
	}

	index := addedAtIndexes(collectAddedAtItems(ctx, client, server, libraryID, sort))
	cache.put(key, index)

	return index
}

// addedAtCacheKey identifies an added-at jump rail by server, library, and sort.
//
// Parameters:
//   - server: PMS identity.
//   - libraryID: Section key.
//   - sort: Normalized Outtake sort key.
//
// Returns:
//   - key: Cache key.
func addedAtCacheKey(server plex.Server, libraryID, sort string) string {
	return server.Address + ":" + strconv.Itoa(server.Port) + "|" + libraryID + "|" + sort
}

// get returns a copy of cached buckets for key.
//
// Parameters:
//   - key: Cache key.
//
// Returns:
//   - index: Cached buckets.
//   - ok: True when the key is present.
func (cache *addedAtIndexCache) get(key string) ([]plex.LetterIndex, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	index, ok := cache.indexes[key]

	return slices.Clone(index), ok
}

// put stores a copy of index for key.
//
// Parameters:
//   - key: Cache key.
//   - index: Month buckets to store.
func (cache *addedAtIndexCache) put(key string, index []plex.LetterIndex) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.indexes == nil {
		cache.indexes = make(map[string][]plex.LetterIndex)
	}

	cache.indexes[key] = slices.Clone(index)
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
			return jumpOtherKey
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
	key := strings.TrimSpace(titleSort)
	if key == "" {
		key = strings.TrimSpace(title)
	}

	if key == "" {
		return jumpOtherKey
	}

	first, _ := utf8.DecodeRuneInString(key)
	upper := unicode.ToUpper(first)
	if upper >= 'A' && upper <= 'Z' {
		return string(upper)
	}

	return jumpOtherKey
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

	for i := range items {
		key := keyFn(items[i])
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
	pages := 0

	for {
		page, err := client.GetMediaPage(
			ctx,
			server,
			libraryID,
			start,
			addedIndexPageSize,
			plexMediaSort(sort),
		)
		if err != nil {
			return items
		}

		items = append(items, page.Items...)

		start += len(page.Items)
		pages++

		if len(page.Items) == 0 || start >= page.Total || pages >= maxAddedIndexPages {
			return items
		}
	}
}
