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
// absent.
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
	seen := make(map[string]struct{}, len(paths))
	slots := make(chan struct{}, max(limit, 1))

	for _, path := range paths {
		if _, repeat := seen[path]; repeat || path == "" {
			continue
		}

		seen[path] = struct{}{}

		slots <- struct{}{}

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
