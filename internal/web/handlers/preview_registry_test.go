// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/media/progress"
	"github.com/PapagoLabs/outtake/internal/queue"
)

// testPreviewID is the id every registry test registers, since each test gets
// its own registry.
const testPreviewID = "p1"

// errRender is a failure standing in for an ffmpeg error.
var errRender = errors.New("ffmpeg exited 1")

// awaitPreview waits for the registry's preview to reach a terminal state.
//
// Every test uses its own registry with a single preview, so the id is fixed.
//
// Parameters:
//   - t: Test context.
//   - registry: Registry holding the preview.
func awaitPreview(t *testing.T, registry *previewRegistry) previewView {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		view, ok := registry.get(testPreviewID)
		require.True(t, ok, "the preview should still be registered")

		if view.done() {
			return view
		}

		if time.Now().After(deadline) {
			t.Fatalf("preview never finished, last status %q at %d%%", view.Status, view.Progress)
		}

		time.Sleep(time.Millisecond)
	}
}

// TestPreviewRegistryRecordsProgress is the point of this branch: the existing
// ffmpeg writer reports into the registry, so a client can read a percentage.
func TestPreviewRegistryRecordsProgress(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	callbacks := make(chan func(int), 1)
	released := make(chan struct{})

	registry.render(t.Context(), "p1", func(ctx context.Context) error {
		// The callback is published rather than exercised here, so the
		// assertions run on the test goroutine. require calls FailNow, which is
		// only legal there.
		callbacks <- progress.From(ctx)

		<-released

		return nil
	})

	var report func(int)

	select {
	case report = <-callbacks:
	case <-time.After(5 * time.Second):
		t.Fatal("the render never published its progress callback")
	}

	require.NotNil(t, report, "the render context must carry a progress callback")

	// This is what ffmpeg's writer does with each parsed time= line.
	for _, percent := range []int{0, 25, 50, 99} {
		report(percent)
	}

	view, ok := registry.get("p1")
	require.True(t, ok)
	assert.Equal(t, 99, view.Progress, "progress should be readable while the render runs")
	assert.False(t, view.done(), "the render should still be in flight")

	close(released)

	final := awaitPreview(t, registry)
	assert.Equal(t, queue.JobStatusCompleted, final.Status)
	assert.Equal(t, previewProgressDone, final.Progress, "a finished render reports 100 percent")
}

func TestPreviewRegistryRecordsFailure(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()

	registry.render(t.Context(), "p1", func(context.Context) error {
		return errRender
	})

	view := awaitPreview(t, registry)
	assert.Equal(t, queue.JobStatusFailed, view.Status)
	assert.Equal(t, errRender.Error(), view.Error)
}

func TestPreviewRegistryRecordsCancellation(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	started := make(chan struct{})

	registry.render(t.Context(), "p1", func(ctx context.Context) error {
		close(started)

		<-ctx.Done()

		return ctx.Err()
	})

	<-started

	require.True(t, registry.cancel("p1"), "a running preview should be cancellable")

	view := awaitPreview(t, registry)
	assert.Equal(t, queue.JobStatusCancelled, view.Status)
	assert.Empty(t, view.Error, "a cancellation is not a failure")
}

// TestPreviewRegistryCancelIgnoresFinished confirms a completed preview cannot
// be canceled, so a late click cannot rewrite a result.
func TestPreviewRegistryCancelIgnoresFinished(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()

	registry.render(t.Context(), "p1", func(context.Context) error { return nil })
	awaitPreview(t, registry)

	assert.False(t, registry.cancel("p1"), "a finished preview should not report canceled")
	assert.False(t, registry.cancel("unknown"), "an unknown id should not report canceled")
}

// TestPreviewRegistryOutlivesRequest is what makes a preview asynchronous: the
// render must continue after the request that started it returns.
func TestPreviewRegistryOutlivesRequest(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()

	requestCtx, endRequest := context.WithCancel(t.Context())
	finished := make(chan struct{})

	registry.render(requestCtx, "p1", func(ctx context.Context) error {
		// The request goes away immediately, as it would for a 202.
		endRequest()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
			close(finished)

			return nil
		}
	})

	view := awaitPreview(t, registry)
	assert.Equal(t, queue.JobStatusCompleted, view.Status,
		"the render must not be canceled by its request finishing")
}

// TestPreviewRegistryClampsProgress keeps a malformed report from escaping the
// 0-100 range the UI renders.
func TestPreviewRegistryClampsProgress(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	held := make(chan struct{})

	registry.render(t.Context(), "p1", func(ctx context.Context) error {
		onProgress := progress.From(ctx)
		onProgress(-40)
		onProgress(500)

		<-held

		return nil
	})

	var seen previewView

	for {
		if view, ok := registry.get("p1"); ok && view.Progress > 0 {
			seen = view

			break
		}

		time.Sleep(time.Millisecond)
	}

	assert.Equal(t, previewProgressDone, seen.Progress, "progress should clamp to 100")

	close(held)
	awaitPreview(t, registry)
}

// TestPreviewRegistryIgnoresProgressGoingBackwards keeps a stale report from
// rewinding the percentage a client is already rendering.
func TestPreviewRegistryIgnoresProgressGoingBackwards(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	released := make(chan struct{})

	registry.render(t.Context(), testPreviewID, func(ctx context.Context) error {
		report := progress.From(ctx)
		report(60)
		report(10)

		<-released

		return nil
	})

	// Wait for the first report to land.
	for {
		view, ok := registry.get(testPreviewID)
		require.True(t, ok)

		if view.Progress > 0 {
			assert.Equal(t, 60, view.Progress, "a lower report must not rewind the percentage")

			break
		}

		time.Sleep(time.Millisecond)
	}

	close(released)
	awaitPreview(t, registry)
}

// TestPreviewRegistryRejectsDuplicateID covers a second submission of a preview
// that is already rendering.
//
// The id is meant to be stable for the same content, so this is the normal case
// once previews are content addressed. Replacing the entry would leave the first
// render's goroutine writing to, and canceling, the second.
func TestPreviewRegistryRejectsDuplicateID(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	firstStarted := make(chan struct{})
	release := make(chan struct{})
	renders := 0

	var mu sync.Mutex

	count := func() int {
		mu.Lock()
		defer mu.Unlock()

		return renders
	}

	require.True(t, registry.render(t.Context(), testPreviewID, func(ctx context.Context) error {
		mu.Lock()

		renders++
		mu.Unlock()

		close(firstStarted)

		<-release

		return ctx.Err()
	}), "the first render should start")

	<-firstStarted

	require.False(t, registry.render(t.Context(), testPreviewID, func(context.Context) error {
		mu.Lock()

		renders++
		mu.Unlock()

		return nil
	}), "a duplicate id should not start a second render")

	assert.Equal(t, 1, count(), "the duplicate render must never run")

	// The first render must still be the one the registry knows about, so
	// canceling it stops that render rather than something else.
	require.True(
		t,
		registry.cancel(testPreviewID),
		"the original render should still be cancellable",
	)

	close(release)

	view := awaitPreview(t, registry)
	assert.Equal(t, queue.JobStatusCancelled, view.Status)
	assert.Equal(t, 1, count(), "only the first render should have run")
}

// TestPreviewRegistryEvictsBehindARunningPreview covers a slow preview sitting
// at the front of the order.
//
// Stopping eviction at the first running entry would let one slow render hold up
// every cleanup behind it. Renders waiting on the preview gate are running too,
// so under a burst of submissions the map would grow well past what is retained.
func TestPreviewRegistryEvictsBehindARunningPreview(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	blocked := make(chan struct{})
	started := make(chan struct{})

	// A preview that never finishes sits at the front of the order.
	require.True(t, registry.render(t.Context(), "blocked", func(context.Context) error {
		close(started)
		<-blocked

		return nil
	}))

	<-started

	renders := previewRetained + 5
	finished := make(chan struct{}, renders)

	for i := range renders {
		registry.render(t.Context(), previewID(i), func(context.Context) error {
			finished <- struct{}{}

			return nil
		})
	}

	for range renders {
		<-finished
	}

	// Only the submitted previews are awaited; the blocked one never finishes,
	// which is the point of it.
	awaitDoneExcept(t, registry, "blocked")

	registry.mu.Lock()
	defer registry.mu.Unlock()

	// The blocked render is still tracked, so it can be canceled and read.
	_, running := registry.jobs["blocked"]
	assert.True(t, running, "a running preview must keep its entry")

	// The finished previews behind it were still reclaimed.
	retained := 0

	for _, id := range registry.order {
		if job, ok := registry.jobs[id]; ok && job.view.done() {
			retained++
		}
	}

	assert.LessOrEqual(t, retained, previewRetained,
		"a running preview at the front must not block eviction of finished ones")

	_, oldestKept := registry.jobs[previewID(0)]
	assert.False(t, oldestKept, "the oldest finished preview should have been dropped")

	close(blocked)
}

// TestPreviewRegistryEvictsOldestFinished bounds the map, which nothing else
// does: a preview is in-memory, and its file is reclaimed separately.
func TestPreviewRegistryEvictsOldestFinished(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()

	// render is asynchronous, so every render is signaled before the bound is
	// checked. A preview still in flight is deliberately exempt from eviction,
	// so asserting without waiting would race with the goroutines.
	renders := previewRetained + 5
	done := make(chan struct{}, renders)

	for i := range renders {
		registry.render(t.Context(), previewID(i), func(context.Context) error {
			done <- struct{}{}

			// Hold the render open after signaling, so there is a real gap
			// between the render function returning and finish recording it.
			// The wait below is then what makes the assertions reliable rather
			// than the goroutines happening to be quick.
			time.Sleep(20 * time.Millisecond)

			return nil
		})
	}

	for range renders {
		<-done
	}

	// A render function returning is not the same as finish having run: finish
	// is called after fn returns, in the same goroutine. Waiting for every
	// registered job to report a terminal status closes that window, so the
	// counts below reflect settled state rather than whatever happens to be
	// mid-flight.
	awaitAllRendersDone(t, registry)

	registry.mu.Lock()
	defer registry.mu.Unlock()

	assert.LessOrEqual(
		t,
		len(registry.jobs),
		previewRetained,
		"the registry grew past what it retains",
	)

	_, oldestKept := registry.jobs[previewID(0)]
	assert.False(t, oldestKept, "the oldest finished preview should have been dropped")
}

// awaitDoneExcept blocks until every registered job other than the named one
// has reached a terminal state.
//
// Parameters:
//   - t: Test context, which supplies the deadline budget.
//   - registry: Registry to inspect.
//   - except: Id to leave out of the wait, for a render that never finishes.
func awaitDoneExcept(t *testing.T, registry *previewRegistry, except string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		registry.mu.Lock()

		pending := 0

		for id, job := range registry.jobs {
			if id != except && !job.view.done() {
				pending++
			}
		}

		registry.mu.Unlock()

		if pending == 0 {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("%d previews never finished", pending)
		}

		time.Sleep(time.Millisecond)
	}
}

// awaitAllRendersDone blocks until every registered job has reached a terminal
// state.
//
// A render function returning does not mean finish has run, since finish is
// called after fn returns in the same goroutine. Counting on the render
// returning alone therefore races the registry.
//
// Parameters:
//   - t: Test context, which supplies the deadline budget.
//   - registry: Registry to inspect.
func awaitAllRendersDone(t *testing.T, registry *previewRegistry) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		registry.mu.Lock()

		pending := 0

		for _, job := range registry.jobs {
			if !job.view.done() {
				pending++
			}
		}

		registry.mu.Unlock()

		if pending == 0 {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("%d previews never finished", pending)
		}

		time.Sleep(time.Millisecond)
	}
}

// TestPreviewRegistryKeepsInFlightDuringEviction guards the eviction order: a
// render still running must not be dropped to make room.
func TestPreviewRegistryKeepsInFlightDuringEviction(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	held := make(chan struct{})

	registry.render(t.Context(), "in-flight", func(ctx context.Context) error {
		<-held

		return nil
	})

	for i := range previewRetained + 5 {
		registry.render(t.Context(), previewID(i), func(context.Context) error { return nil })
	}

	_, ok := registry.get("in-flight")
	assert.True(t, ok, "a running preview must keep its place")

	close(held)
}

// TestPreviewRegistryConcurrentProgress covers reports arriving from the
// ffmpeg writer while status is being read and written.
func TestPreviewRegistryConcurrentProgress(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	held := make(chan struct{})

	registry.render(t.Context(), "p1", func(ctx context.Context) error {
		onProgress := progress.From(ctx)

		var wg sync.WaitGroup

		for range 4 {
			wg.Go(func() {
				for percent := range 100 {
					onProgress(percent)
				}
			})
		}

		wg.Wait()
		<-held

		return nil
	})

	var wg sync.WaitGroup

	for range 4 {
		wg.Go(func() {
			for range 200 {
				registry.get("p1")
			}
		})
	}

	wg.Wait()

	close(held)
	awaitPreview(t, registry)
}

// previewID builds a stable id for a test preview.
//
// Parameters:
//   - i: Ordinal.
//
// Returns:
//   - id: The preview id.
func previewID(i int) string {
	return "p" + string(rune('a'+i%26)) + "-" + time.Duration(i).String()
}
