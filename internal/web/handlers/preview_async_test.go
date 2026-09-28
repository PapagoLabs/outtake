// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/storage"
)

// statusResponse reads a preview status response.
type statusResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	URL      string `json:"url"`
	Error    string `json:"error"`
}

// newAsyncHandler builds a handler whose previews render in the background.
func newAsyncHandler(t *testing.T) (*ClipHandler, *storage.Storage) {
	t.Helper()

	store, err := storage.NewStorage(t.TempDir())
	require.NoError(t, err)

	handler := NewClipHandler(
		nil,
		store,
		nil,
		&config.Config{MaxConcurrentPreviews: 2, FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"},
		nil,
		"outtake",
		"test",
	)

	return handler, store
}

// getStatus calls the status route for one preview.
func getStatus(t *testing.T, handler *ClipHandler, id string) (int, statusResponse) {
	t.Helper()

	app := fiber.New()
	app.Get("/api/clips/preview/:id", handler.PreviewStatus)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/clips/preview/"+id, nil,
	))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	var body statusResponse

	if resp.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	}

	return resp.StatusCode, body
}

// TestPreviewStatusReportsAQueuedRender covers the status endpoint while a
// render is still in flight, which is what a client polls.
//
// The render is a bare blocking closure rather than a real encode: the ffmpeg
// path is covered by the staging tests, and building a mock inside the render
// goroutine would use the test's own state off the test goroutine.
func TestPreviewStatusReportsAQueuedRender(t *testing.T) {
	t.Parallel()

	handler, _ := newAsyncHandler(t)

	started := make(chan struct{})
	release := make(chan struct{})

	require.True(t, handler.previewJobs.render(t.Context(), "p1", func(context.Context) error {
		close(started)
		<-release

		return nil
	}))

	<-started

	view, ok := handler.previewJobs.get("p1")
	require.True(t, ok)
	assert.Equal(t, "processing", string(view.Status))
	assert.Empty(t, view.Error)

	close(release)
}

// TestPreviewStatusReportsARenderWaitingForASlot covers a render waiting for a
// slot.
//
// With every preview slot taken, a further preview is accepted and waits rather
// than being rejected, and it must read as processing throughout: a client
// watching it has to see a render that is underway, not one that errored.
func TestPreviewStatusReportsARenderWaitingForASlot(t *testing.T) {
	t.Parallel()

	handler, _ := newAsyncHandler(t)

	var (
		mu      sync.Mutex
		holders int
	)

	release := make(chan struct{})

	occupy := func() {
		mu.Lock()

		holders++

		acquired, err := handler.previews.acquire(t.Context(), previewWait)
		mu.Unlock()
		require.NoError(t, err)

		<-release

		acquired()
	}

	var occupying sync.WaitGroup

	for range 2 {
		occupy := occupy
		occupying.Go(occupy)
	}

	// Wait for both slots to be held.
	deadline := time.Now().Add(5 * time.Second)

	for {
		mu.Lock()

		held := holders == 2
		mu.Unlock()

		if held {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("the preview slots were never both taken")
		}

		time.Sleep(time.Millisecond)
	}

	waiting := make(chan struct{})

	require.True(
		t,
		handler.previewJobs.render(t.Context(), "queued", func(ctx context.Context) error {
			close(waiting)
			<-ctx.Done()

			return ctx.Err()
		}),
	)

	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued render never started waiting for a slot")
	}

	status, body := getStatus(t, handler, "queued")
	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, "processing", body.Status, "a render waiting for a slot is still processing")

	close(release)
	occupying.Wait()

	// The queued render ends on cancellation rather than on a slot: letting it
	// acquire one would start a real encode of a source that does not exist.
	require.True(t, handler.previewJobs.cancel("queued"))
	awaitTerminal(t, handler.previewJobs, "queued")
}

// TestPreviewStatusSeesACachedPreview covers the cached branch of Preview.
//
// A preview already on disk is returned without a render, so the id has to be
// recorded as finished. Otherwise the page it redirects to polls an id nothing
// is registered under and reports the preview as unknown.
func TestPreviewStatusSeesACachedPreview(t *testing.T) {
	t.Parallel()

	handler, store := newAsyncHandler(t)

	require.NoError(t, os.MkdirAll(
		filepath.Join(store.BasePath(), "previews"), 0o750,
	))
	require.NoError(t, os.WriteFile(store.PreviewPath("cached"), []byte("x"), 0o600))

	// The branch Preview takes for a file already on disk.
	handler.previewJobs.remember("cached")

	status, body := getStatus(t, handler, "cached")
	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, "completed", body.Status)
	assert.Equal(t, "/previews/cached", body.URL)
}

// TestCancelPreviewUnknownIDIsNotFound covers the split between an id nothing
// was registered under and one whose render has already finished.
func TestCancelPreviewUnknownIDIsNotFound(t *testing.T) {
	t.Parallel()

	handler, _ := newAsyncHandler(t)

	app := fiber.New()
	app.Delete("/api/clips/preview/:id", handler.CancelPreview)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodDelete, "/api/clips/preview/never-rendered", nil,
	))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// TestPreviewRegistryAllowsRetryingAFailedRender covers a render that finished
// without leaving a preview.
//
// The id is the same for the same parameters, so rejecting it on the strength of
// the earlier outcome would mean the preview can never be requested again and the
// page stays stranded on a failure.
func TestPreviewRegistryAllowsRetryingAFailedRender(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()

	require.True(t, registry.render(t.Context(), "p1", func(context.Context) error {
		return errRender
	}), "the first attempt should start")

	view := awaitTerminal(t, registry, "p1")
	require.Equal(t, "failed", string(view.Status))

	ran := make(chan struct{})

	require.True(t, registry.render(t.Context(), "p1", func(context.Context) error {
		close(ran)

		return nil
	}), "a failed render must be retryable")

	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the retry never ran")
	}

	retry := awaitTerminal(t, registry, "p1")
	assert.Equal(t, "completed", string(retry.Status))
}

// TestPreviewRegistryRejectsARunningRender is the counterpart: a render in
// flight keeps its entry, so a second submission joins it rather than replacing
// it.
func TestPreviewRegistryRejectsARunningRender(t *testing.T) {
	t.Parallel()

	registry := newPreviewRegistry()
	release := make(chan struct{})

	require.True(t, registry.render(t.Context(), "p1", func(ctx context.Context) error {
		<-ctx.Done()

		return ctx.Err()
	}))

	require.False(t, registry.render(t.Context(), "p1", func(context.Context) error {
		close(release)

		return nil
	}), "a render in flight must keep its entry")

	assert.True(t, registry.cancel("p1"))
}

// awaitTerminal blocks until a preview reaches a terminal status.
//
// Polling a fixed number of times is a flake on a loaded machine, and a render
// closure returning does not mean the registry has recorded the outcome yet, so
// the wait is on the recorded status rather than on the render.
//
// Parameters:
//   - t: Test context.
//   - registry: Registry holding the preview.
//   - id: Preview id to wait on.
//
// Returns:
//   - view: The terminal state.
func awaitTerminal(t *testing.T, registry *previewRegistry, id string) previewView {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		view, ok := registry.get(id)
		require.True(t, ok, "the preview should still be registered")

		if view.done() {
			return view
		}

		if time.Now().After(deadline) {
			t.Fatalf(
				"preview never reached a terminal status, last was %q at %d%%",
				view.Status,
				view.Progress,
			)
		}

		time.Sleep(time.Millisecond)
	}
}

// TestPreviewStatusOmitsURLUntilPublished covers the reason a client cannot be
// handed a URL to something that is not there yet.
func TestPreviewStatusOmitsURLUntilPublished(t *testing.T) {
	t.Parallel()

	handler, store := newAsyncHandler(t)

	handler.previewJobs.render(t.Context(), "p1", func(context.Context) error { return nil })

	awaitTerminal(t, handler.previewJobs, "p1")

	// Completed, but nothing was published, so there is no file to serve.
	_, body := getStatus(t, handler, "p1")
	assert.Equal(t, "completed", body.Status)
	assert.Empty(t, body.URL, "a preview with no published file must not be offered a URL")

	// Publishing one makes the URL appear.
	require.NoError(t, os.WriteFile(store.PreviewPath("p1"), []byte("x"), 0o600))

	_, body = getStatus(t, handler, "p1")
	assert.Equal(t, "/previews/p1", body.URL)
}

// TestPreviewStatusUnknownIsNotFound covers a poll for an id nothing is
// registered under.
func TestPreviewStatusUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	handler, _ := newAsyncHandler(t)

	status, _ := getStatus(t, handler, "never-rendered")

	assert.Equal(t, fiber.StatusNotFound, status)
}

// TestCancelPreviewStopsARunningRender covers cancellation through the route.
func TestCancelPreviewStopsARunningRender(t *testing.T) {
	t.Parallel()

	handler, _ := newAsyncHandler(t)

	running := make(chan struct{})
	finished := make(chan struct{})

	handler.previewJobs.render(t.Context(), "p1", func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		close(finished)

		return ctx.Err()
	})

	<-running

	app := fiber.New()
	app.Delete("/api/clips/preview/:id", handler.CancelPreview)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodDelete, "/api/clips/preview/p1", nil,
	))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("canceling did not reach the render")
	}

	// The closure signals before returning, so the recorded status is waited on
	// rather than assumed.
	view := awaitTerminal(t, handler.previewJobs, "p1")
	assert.Equal(t, "canceled", string(view.Status))
}

// TestCancelPreviewRejectsAFinishedRender covers a late cancel arriving after
// the render already ended, which must not report success.
func TestCancelPreviewRejectsAFinishedRender(t *testing.T) {
	t.Parallel()

	handler, _ := newAsyncHandler(t)

	handler.previewJobs.render(t.Context(), "p1", func(context.Context) error { return nil })

	awaitTerminal(t, handler.previewJobs, "p1")

	app := fiber.New()
	app.Delete("/api/clips/preview/:id", handler.CancelPreview)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodDelete, "/api/clips/preview/p1", nil,
	))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)
}
