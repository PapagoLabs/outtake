// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

// TestPreviewStatusOmitsURLUntilPublished covers the reason a client cannot be
// handed a URL to something that is not there yet.
func TestPreviewStatusOmitsURLUntilPublished(t *testing.T) {
	t.Parallel()

	handler, store := newAsyncHandler(t)

	handler.previewJobs.render(t.Context(), "p1", func(context.Context) error { return nil })

	for range 200 {
		view, ok := handler.previewJobs.get("p1")
		require.True(t, ok)

		if view.done() {
			break
		}

		time.Sleep(time.Millisecond)
	}

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

	view, ok := handler.previewJobs.get("p1")
	require.True(t, ok)
	assert.Equal(t, "canceled", string(view.Status))
}

// TestCancelPreviewRejectsAFinishedRender covers a late cancel arriving after
// the render already ended, which must not report success.
func TestCancelPreviewRejectsAFinishedRender(t *testing.T) {
	t.Parallel()

	handler, _ := newAsyncHandler(t)

	handler.previewJobs.render(t.Context(), "p1", func(context.Context) error { return nil })

	for range 200 {
		view, ok := handler.previewJobs.get("p1")
		require.True(t, ok)

		if view.done() {
			break
		}

		time.Sleep(time.Millisecond)
	}

	app := fiber.New()
	app.Delete("/api/clips/preview/:id", handler.CancelPreview)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodDelete, "/api/clips/preview/p1", nil,
	))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)
}
