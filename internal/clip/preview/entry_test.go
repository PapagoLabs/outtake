// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/progress"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

var errRender = errors.New("ffmpeg exited 1")

var errKilledProcess = errors.New("extract preview: ffmpeg: signal: killed")

func awaitPreview(t *testing.T, registry *registry) View {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		view, ok := registry.resolve("p1")
		require.True(t, ok, "the preview should still be registered")

		if view.Done() {
			return view
		}

		if time.Now().After(deadline) {
			t.Fatalf("preview never finished, last status %q at %d%%", view.Status, view.Progress)
		}

		time.Sleep(time.Millisecond)
	}
}

func TestPreviewRegistryRecordsProgress(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})
	callbacks := make(chan func(int), 1)
	released := make(chan struct{})

	registry.render(t.Context(), "p1", 4, func(ctx context.Context) error {
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

	for _, percent := range []int{0, 25, 50, 99} {
		report(percent)
	}

	view, ok := registry.resolve("p1")
	require.True(t, ok)
	assert.Equal(t, 99, view.Progress, "progress should be readable while the render runs")
	assert.False(t, view.Done(), "the render should still be in flight")

	close(released)

	final := awaitPreview(t, registry)
	assert.Equal(t, clip.StatusCompleted, final.Status)
	assert.Equal(t, progressDone, final.Progress, "a finished render reports 100 percent")
}

func TestPreviewRegistryRecordsFailure(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})

	registry.render(t.Context(), "p1", 4, func(context.Context) error {
		return errRender
	})

	view := awaitPreview(t, registry)
	assert.Equal(t, clip.StatusFailed, view.Status)
	assert.Equal(t, errRender.Error(), view.Error)
}

// TestPreviewRegistryDescribesAFailure covers the describer a service was
// given: a failed preview shows its message and details, and the describer is
// told which preview failed.
func TestPreviewRegistryDescribesAFailure(t *testing.T) {
	t.Parallel()

	service := New(1, nil, blob.Paths{}, nil)

	var (
		subject string
		got     error
	)

	service.SetFailureFunc(func(what string, err error) clip.Failure {
		subject, got = what, err

		return clip.Failure{
			Message: "Plain words",
			Details: "the report",
			Ref:     "abc123",
			Chain:   err.Error(),
		}
	})

	service.entries.render(t.Context(), "p1", 4, func(context.Context) error {
		return errRender
	})

	view := awaitPreview(t, service.entries)
	assert.Equal(t, clip.StatusFailed, view.Status)
	assert.Equal(t, "Plain words", view.Error)
	assert.Equal(t, "the report", view.ErrorDetails)
	assert.Equal(t, "preview p1", subject)
	require.ErrorIs(t, got, errRender)
}

func TestPreviewRegistryRecordsCancellation(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})
	started := make(chan struct{})

	registry.render(t.Context(), "p1", 4, func(ctx context.Context) error {
		close(started)

		<-ctx.Done()

		return ctx.Err()
	})

	<-started

	require.True(t, registry.cancel("p1"), "a running preview should be cancellable")

	view := awaitPreview(t, registry)
	assert.Equal(t, clip.StatusCancelled, view.Status)
	assert.Empty(t, view.Error, "a cancellation is not a failure")
}

func TestPreviewRegistryCancelIgnoresFinished(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})

	registry.render(t.Context(), "p1", 4, func(context.Context) error { return nil })
	awaitPreview(t, registry)

	assert.False(t, registry.cancel("p1"), "a finished preview should not report canceled")
	assert.False(t, registry.cancel("unknown"), "an unknown id should not report canceled")
}

func TestPreviewRegistryOutlivesRequest(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})

	requestCtx, endRequest := context.WithCancel(t.Context())
	finished := make(chan struct{})

	registry.render(requestCtx, "p1", 4, func(ctx context.Context) error {
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
	assert.Equal(t, clip.StatusCompleted, view.Status,
		"the render must not be canceled by its request finishing")
}

func TestPreviewRegistryClampsProgress(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})
	held := make(chan struct{})

	registry.render(t.Context(), "p1", 4, func(ctx context.Context) error {
		onProgress := progress.From(ctx)
		onProgress(-40)
		onProgress(500)

		<-held

		return nil
	})

	var seen View

	for {
		if view, ok := registry.resolve("p1"); ok && view.Progress > 0 {
			seen = view

			break
		}

		time.Sleep(time.Millisecond)
	}

	assert.Equal(t, progressDone, seen.Progress, "progress should clamp to 100")

	close(held)
	awaitPreview(t, registry)
}

func TestPreviewRegistryIgnoresProgressGoingBackwards(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})
	released := make(chan struct{})

	registry.render(t.Context(), "p1", 4, func(ctx context.Context) error {
		report := progress.From(ctx)
		report(60)
		report(10)

		<-released

		return nil
	})

	for {
		view, ok := registry.resolve("p1")
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

func TestPreviewRegistryRejectsDuplicateID(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})
	firstStarted := make(chan struct{})
	release := make(chan struct{})
	renders := 0

	var mu sync.Mutex

	count := func() int {
		mu.Lock()
		defer mu.Unlock()

		return renders
	}

	require.Equal(
		t,
		AdmittedRender,
		registry.render(t.Context(), "p1", 4, func(ctx context.Context) error {
			mu.Lock()

			renders++

			mu.Unlock()

			close(firstStarted)

			<-release

			return ctx.Err()
		}),
		"the first render should start",
	)

	<-firstStarted

	require.Equal(
		t,
		AdmittedExisting,
		registry.render(t.Context(), "p1", 4, func(context.Context) error {
			mu.Lock()

			renders++

			mu.Unlock()

			return nil
		}),
		"a duplicate id should not start a second render",
	)

	assert.Equal(t, 1, count(), "the duplicate render must never run")

	require.True(
		t,
		registry.cancel("p1"),
		"the original render should still be cancellable",
	)

	close(release)

	view := awaitPreview(t, registry)
	assert.Equal(t, clip.StatusCancelled, view.Status)
	assert.Equal(t, 1, count(), "only the first render should have run")
}

func TestPreviewRegistryEvictsBehindARunningPreview(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})
	blocked := make(chan struct{})
	started := make(chan struct{})

	slots := ((retained + 5) / queuedPerSlot) + 1

	require.Equal(
		t,
		AdmittedRender,
		registry.render(t.Context(), "blocked", slots, func(context.Context) error {
			close(started)
			<-blocked

			return nil
		}),
	)

	<-started

	renders := retained + 5
	finished := make(chan struct{}, renders)

	for i := range renders {
		registry.render(t.Context(), previewID(i), slots, func(context.Context) error {
			finished <- struct{}{}

			return nil
		})
	}

	for range renders {
		<-finished
	}

	awaitDoneExcept(t, registry, "blocked")

	registry.mu.Lock()
	defer registry.mu.Unlock()

	_, running := registry.entries["blocked"]
	assert.True(t, running, "a running preview must keep its entry")

	kept := 0

	for _, tracked := range registry.order {
		if job, ok := registry.entries[tracked]; ok && job.view.Done() {
			kept++
		}
	}

	assert.LessOrEqual(t, kept, retained,
		"a running preview at the front must not block eviction of finished ones")

	_, oldestKept := registry.entries[previewID(0)]
	assert.False(t, oldestKept, "the oldest finished preview should have been dropped")

	close(blocked)
}

func TestPreviewRegistryEvictsOldestFinished(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})

	renders := retained + 5
	done := make(chan struct{}, renders)

	slots := (renders / queuedPerSlot) + 1

	for i := range renders {
		registry.render(t.Context(), previewID(i), slots, func(context.Context) error {
			done <- struct{}{}

			time.Sleep(20 * time.Millisecond)

			return nil
		})
	}

	for range renders {
		<-done
	}

	awaitAllRendersDone(t, registry)

	registry.mu.Lock()
	defer registry.mu.Unlock()

	assert.LessOrEqual(
		t,
		len(registry.entries),
		retained,
		"the registry grew past what it retains",
	)

	_, oldestKept := registry.entries[previewID(0)]
	assert.False(t, oldestKept, "the oldest finished preview should have been dropped")
}

func awaitDoneExcept(t *testing.T, registry *registry, except string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		registry.mu.Lock()

		pending := 0

		for tracked, job := range registry.entries {
			if tracked != except && !job.view.Done() {
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

func awaitAllRendersDone(t *testing.T, registry *registry) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		registry.mu.Lock()

		pending := 0

		for _, job := range registry.entries {
			if !job.view.Done() {
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

func TestPreviewRegistryKeepsInFlightDuringEviction(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})
	held := make(chan struct{})

	registry.render(t.Context(), "in-flight", 4, func(ctx context.Context) error {
		<-held

		return nil
	})

	for i := range retained + 5 {
		registry.render(
			t.Context(),
			previewID(i),
			4,
			func(context.Context) error { return nil },
		)
	}

	_, ok := registry.resolve("in-flight")
	assert.True(t, ok, "a running preview must keep its place")

	close(held)
}

func TestPreviewRegistryConcurrentProgress(t *testing.T) {
	t.Parallel()

	registry := newRegistry(func(string) {})
	held := make(chan struct{})

	registry.render(t.Context(), "p1", 4, func(ctx context.Context) error {
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
				registry.resolve("p1")
			}
		})
	}

	wg.Wait()

	close(held)
	awaitPreview(t, registry)
}

func previewID(i int) string {
	return "p" + string(rune('a'+i%26)) + "-" + time.Duration(i).String()
}
