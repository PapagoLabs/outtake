// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/PapagoLabs/outtake/internal/plex"
)

// LetterIndex is one jump-rail target on a library section.
type LetterIndex struct {
	Title string
	Size  int
}

// AddedAtCache stores added-at jump buckets by server, library, and sort.
type AddedAtCache struct {
	mu      sync.Mutex
	indexes map[string][]plex.LetterIndex
}

const (
	// addedIndexPageSize is the PMS page size buckets are built from.
	addedIndexPageSize = 200
	// maxAddedIndexPages bounds added-at jump-rail collection.
	maxAddedIndexPages = 10

	// maxJumpLabels is how many marks a crowded rail is cut to.
	maxJumpLabels = 28

	// yearTickDenseMax is the bucket count past which year ticks step by five.
	yearTickDenseMax = 70
	// yearTickDenseStep keeps every fifth year on a moderately long rail.
	yearTickDenseStep = 5
	// yearTickSparseStep keeps every tenth year on a long rail.
	yearTickSparseStep = 10
	// monthYearLen is the trailing year length in an mm/yyyy label.
	monthYearLen = 4
	// jumpSampleEnds is how many marks are always kept when sampling a rail.
	jumpSampleEnds = 2
)

// OrderIndex sorts or reverses buckets to match the active sort.
//
// Parameters:
//   - buckets: PMS buckets in default order.
//   - sortKey: Normalized Outtake sort key.
//
// Returns:
//   - buckets: Buckets ordered for the jump rail.
func OrderIndex(buckets []plex.LetterIndex, sortKey string) []plex.LetterIndex {
	switch sortKey {
	case SortTitleDesc:
		return plex.ReverseIndexes(buckets)
	case SortYearDesc:
		return plex.ReverseIndexes(plex.SortYearIndexes(buckets))
	case SortYearAsc:
		return plex.SortYearIndexes(buckets)
	default:
		return buckets
	}
}

// JumpMarks converts PMS buckets into a readable set of nice ticks.
//
// Parameters:
//   - buckets: PMS first-character, year, or added-at buckets.
//   - sortKey: Normalized Outtake sort key.
//
// Returns:
//   - marks: Jump-rail targets for the active sort.
func JumpMarks(buckets []plex.LetterIndex, sortKey string) []LetterIndex {
	return thinJumpMarks(toJumpMarks(buckets), sortKey)
}

// TitleIndexes builds first-character buckets from a sorted library listing.
//
// Parameters:
//   - items: Media items in title order.
//
// Returns:
//   - index: Consecutive letter buckets.
func TitleIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return groupIndexes(items, func(item plex.MediaItem) string {
		return TitleJumpKey(item.TitleSort, item.Title)
	})
}

// YearIndexes builds year buckets from a sorted library listing.
//
// Parameters:
//   - items: Media items in year order.
//
// Returns:
//   - index: Consecutive year buckets.
func YearIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return groupIndexes(items, func(item plex.MediaItem) string {
		if item.Year <= 0 {
			return JumpOtherKey
		}

		return strconv.Itoa(item.Year)
	})
}

// AddedAtIndexes builds mm/yyyy buckets from a sorted library listing.
//
// Parameters:
//   - items: Media items in added-at order.
//
// Returns:
//   - index: Consecutive month buckets.
func AddedAtIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return groupIndexes(items, func(item plex.MediaItem) string {
		return addedMonthKey(item.AddedAt)
	})
}

// Load returns the cached added-at buckets, collecting them on a miss.
//
// Parameters:
//   - ctx: Request context.
//   - client: PMS client.
//   - server: PMS to query.
//   - libraryID: Section key.
//   - sortKey: Normalized Outtake sort key.
//
// Returns:
//   - index: Month buckets for the jump rail.
func (cache *AddedAtCache) Load(
	ctx context.Context,
	client *plex.Client,
	server plex.Server,
	libraryID, sortKey string,
) []plex.LetterIndex {
	key := addedAtCacheKey(server, libraryID, sortKey)
	if index, ok := cache.get(key); ok {
		return index
	}

	index := AddedAtIndexes(collectAddedAtItems(ctx, client, server, libraryID, sortKey))
	cache.put(key, index)

	return index
}

// get returns a copy of cached buckets for key.
//
// Parameters:
//   - key: Cache key.
//
// Returns:
//   - index: Cached buckets.
//   - ok: True when the key is present.
func (cache *AddedAtCache) get(key string) ([]plex.LetterIndex, bool) {
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
func (cache *AddedAtCache) put(key string, index []plex.LetterIndex) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.indexes == nil {
		cache.indexes = make(map[string][]plex.LetterIndex)
	}

	cache.indexes[key] = slices.Clone(index)
}

// toJumpMarks maps PMS first-character buckets onto rail marks.
//
// Parameters:
//   - buckets: PMS first-character directories.
//
// Returns:
//   - marks: Non-empty letters with their item counts.
func toJumpMarks(buckets []plex.LetterIndex) []LetterIndex {
	marks := make([]LetterIndex, 0, len(buckets))

	for _, entry := range buckets {
		if entry.Size > 0 {
			marks = append(marks, LetterIndex{
				Title: entry.Title,
				Size:  entry.Size,
			})
		}
	}

	return marks
}

// thinJumpMarks reduces rail marks to a readable set of nice ticks.
//
// Parameters:
//   - marks: Jump targets for the active sort.
//   - sortKey: Normalized Outtake sort key.
//
// Returns:
//   - marks: A thinned list of rail marks.
func thinJumpMarks(marks []LetterIndex, sortKey string) []LetterIndex {
	if len(marks) <= maxJumpLabels {
		return marks
	}

	switch sortKey {
	case SortYearDesc, SortYearAsc:
		return thinNumericMarks(marks, yearTickStep(len(marks)))
	case SortAddedDesc, SortAddedAsc:
		return thinMonthMarks(marks)
	default:
		return marks
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

// thinNumericMarks keeps first, last, and step-aligned numeric titles.
//
// Parameters:
//   - marks: Numeric jump targets in display order.
//   - step: Year modulus to keep.
//
// Returns:
//   - marks: Thinned numeric marks.
func thinNumericMarks(marks []LetterIndex, step int) []LetterIndex {
	if step <= 1 || len(marks) <= jumpSampleEnds {
		return marks
	}

	out := make([]LetterIndex, 0, maxJumpLabels)
	last := len(marks) - 1

	for i, mark := range marks {
		if keepNumericMark(i, last, mark.Title, step) {
			out = append(out, mark)
		}
	}

	if len(out) > maxJumpLabels {
		return sampleJumpMarks(out, maxJumpLabels)
	}

	return out
}

// keepNumericMark reports whether a numeric jump mark should stay on the rail.
//
// Parameters:
//   - position: Position in the ordered year list.
//   - last: Last index in the list.
//   - title: Year label.
//   - step: Year modulus to keep.
//
// Returns:
//   - ok: True for the ends or a step-aligned year.
func keepNumericMark(position, last int, title string, step int) bool {
	if position == 0 || position == last {
		return true
	}

	year, err := strconv.Atoi(title)
	if err != nil {
		return false
	}

	return year%step == 0
}

// thinMonthMarks keeps one month mark per year, then samples if still long.
//
// Parameters:
//   - marks: Month jump targets in display order.
//
// Returns:
//   - marks: Thinned month marks.
func thinMonthMarks(marks []LetterIndex) []LetterIndex {
	if len(marks) <= maxJumpLabels {
		return marks
	}

	yearly := make([]LetterIndex, 0)
	lastYear := ""

	for _, mark := range marks {
		year := monthJumpYear(mark.Title)
		if year == lastYear && len(yearly) > 0 {
			continue
		}

		yearly = append(yearly, mark)
		lastYear = year
	}

	if len(yearly) <= maxJumpLabels {
		return yearly
	}

	return sampleJumpMarks(yearly, maxJumpLabels)
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

// sampleJumpMarks picks evenly spaced marks including the ends.
//
// Parameters:
//   - marks: Jump targets in display order.
//   - limit: Maximum number of marks to keep.
//
// Returns:
//   - marks: Sampled marks.
func sampleJumpMarks(marks []LetterIndex, limit int) []LetterIndex {
	if len(marks) <= limit || limit < jumpSampleEnds {
		return marks
	}

	last := len(marks) - 1
	out := []LetterIndex{marks[0]}

	out = appendInnerJumpMarks(out, marks, limit-jumpSampleEnds, last)

	if out[len(out)-1].Title != marks[last].Title {
		out = append(out, marks[last])
	}

	return out
}

// appendInnerJumpMarks adds evenly spaced interior marks between the ends.
//
// Parameters:
//   - out: Marks already kept, starting with the first letter.
//   - marks: Jump targets in display order.
//   - inner: Number of interior marks to attempt.
//   - last: Last index in marks.
//
// Returns:
//   - out: Marks with interior samples appended.
func appendInnerJumpMarks(
	out, marks []LetterIndex,
	inner, last int,
) []LetterIndex {
	for i := 1; i <= inner; i++ {
		idx := i * last / (inner + 1)
		if idx <= 0 || idx >= last || out[len(out)-1].Title == marks[idx].Title {
			continue
		}

		out = append(out, marks[idx])
	}

	return out
}

// addedMonthKey formats a Plex addedAt unix timestamp as mm/yyyy.
//
// Parameters:
//   - addedAt: Unix seconds, or 0 when unknown.
//
// Returns:
//   - key: Month label, or JumpOtherKey when addedAt is unset.
func addedMonthKey(addedAt int64) string {
	if addedAt <= 0 {
		return JumpOtherKey
	}

	return time.Unix(addedAt, 0).Local().Format("01/2006")
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
//   - sortKey: Normalized Outtake sort key.
//
// Returns:
//   - items: Collected media items, or a prefix when a page fails.
func collectAddedAtItems(
	ctx context.Context,
	client *plex.Client,
	server plex.Server,
	libraryID, sortKey string,
) []plex.MediaItem {
	items := make([]plex.MediaItem, 0)
	start := 0
	collected := 0

	for {
		listing, err := client.GetMediaPage(
			ctx,
			server,
			libraryID,
			start,
			addedIndexPageSize,
			PlexSort(sortKey),
		)
		if err != nil {
			return items
		}

		items = append(items, listing.Items...)

		start += len(listing.Items)
		collected++

		if len(listing.Items) == 0 || start >= listing.Total || collected >= maxAddedIndexPages {
			return items
		}
	}
}

// addedAtCacheKey identifies an added-at jump rail by server, library, and sort.
//
// Parameters:
//   - server: PMS identity.
//   - libraryID: Section key.
//   - sortKey: Normalized Outtake sort key.
//
// Returns:
//   - key: Cache key.
func addedAtCacheKey(server plex.Server, libraryID, sortKey string) string {
	return server.Address + ":" + strconv.Itoa(server.Port) + "|" + libraryID + "|" + sortKey
}
