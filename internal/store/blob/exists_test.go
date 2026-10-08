// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestExistsEachAnswersEveryPathOnce covers the answers: each distinct path is
// checked once, an empty one never, and the lookup reads each result.
func TestExistsEachAnswersEveryPathOnce(t *testing.T) {
	t.Parallel()

	var (
		mu      sync.Mutex
		checked []string
	)

	exists := func(_ context.Context, path string) bool {
		mu.Lock()

		checked = append(checked, path)
		mu.Unlock()

		return path == "/out/a.mp4"
	}

	found := ExistsEach(
		t.Context(),
		exists,
		[]string{"/out/a.mp4", "", "/out/b.mp4", "/out/a.mp4"},
		2,
	)

	assert.True(t, found("/out/a.mp4"))
	assert.False(t, found("/out/b.mp4"))
	assert.False(t, found(""), "an empty path is never present")
	assert.False(t, found("/out/never-asked.mp4"))
	assert.ElementsMatch(
		t,
		[]string{"/out/a.mp4", "/out/b.mp4"},
		checked,
		"each distinct path is checked once",
	)
}

// TestExistsEachRunsChecksAtOnceUpToTheLimit covers the concurrency: slow
// checks overlap, and never more of them than the limit.
func TestExistsEachRunsChecksAtOnceUpToTheLimit(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var running, peak atomic.Int32

		exists := func(context.Context, string) bool {
			now := running.Add(1)
			for {
				seen := peak.Load()
				if now <= seen || peak.CompareAndSwap(seen, now) {
					break
				}
			}

			time.Sleep(time.Second)
			running.Add(-1)

			return true
		}

		paths := []string{"/1", "/2", "/3", "/4", "/5", "/6", "/7", "/8", "/9", "/10"}

		start := time.Now()

		ExistsEach(t.Context(), exists, paths, 3)

		assert.Equal(t, int32(3), peak.Load(), "no more checks than the limit run at once")
		assert.Equal(t, 4*time.Second, time.Since(start), "ten one-second checks, three at a time")
	})
}
