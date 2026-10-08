// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

import (
	"context"
	"sync"
)

// ExistsLimit is how many existence checks ExistsEach runs at once. It bounds
// the HEAD requests a page of clip cards sends to an object store.
const ExistsLimit = 8

// ExistsEach checks many paths at once, at most limit at a time, and returns
// a lookup over the answers. An empty path is never checked and reads as
// absent. Once ctx ends no further check starts, the ones already running
// finish, and every path left unchecked reads as absent.
//
// Parameters:
//   - ctx: Request scope for the checks.
//   - exists: The check to run, such as a Blob's Exists.
//   - paths: Paths to check, which may repeat.
//   - limit: How many checks may run at once, at least one.
//
// Returns:
//   - found: Reports the answer for a checked path, false for any other.
func ExistsEach(
	ctx context.Context,
	exists func(context.Context, string) bool,
	paths []string,
	limit int,
) func(string) bool {
	var (
		mu    sync.Mutex
		group sync.WaitGroup
	)

	answers := make(map[string]bool, len(paths))
	slots := make(chan struct{}, max(limit, 1))

	for _, path := range distinctPaths(paths) {
		if !takeSlot(ctx, slots) {
			break
		}

		group.Go(func() {
			defer func() { <-slots }()

			found := exists(ctx, path)

			mu.Lock()

			answers[path] = found
			mu.Unlock()
		})
	}

	group.Wait()

	return func(path string) bool {
		return answers[path]
	}
}

// distinctPaths drops empty and repeated paths, keeping the first of each.
//
// Parameters:
//   - paths: Paths that may repeat.
//
// Returns:
//   - distinct: Each non-empty path once, in order.
func distinctPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	distinct := make([]string, 0, len(paths))

	for _, path := range paths {
		if _, repeat := seen[path]; repeat || path == "" {
			continue
		}

		seen[path] = struct{}{}
		distinct = append(distinct, path)
	}

	return distinct
}

// takeSlot waits for a free slot, giving up when ctx ends.
//
// Parameters:
//   - ctx: Request scope the wait follows.
//   - slots: Semaphore with one entry per running check.
//
// Returns:
//   - taken: False when ctx ended first.
func takeSlot(ctx context.Context, slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}
