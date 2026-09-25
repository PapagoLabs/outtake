// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/queue"
)

// htmxNoSwap lists the response statuses htmx refuses to swap, mirroring the
// default in internal/web/assets/js/htmx.min.js. A delete response has to stay
// off this list or the card's hx-swap="delete" never fires.
var htmxNoSwap = []int{fiber.StatusNoContent, fiber.StatusNotModified}

// noopJobHandler satisfies the queue without encoding anything.
//
// Parameters:
//   - ctx: Job context.
//   - job: Job to handle.
//
// Returns:
//   - error: Always nil.
func noopJobHandler(_ context.Context, _ *queue.Job) error {
	return nil
}

// newDeleteTestHandler builds a ClipHandler backed by a throwaway database, with
// one clip stored under the given id.
//
// Parameters:
//   - t: Test context.
//   - id: Clip id to seed.
//
// Returns:
//   - handler: A handler ready to serve the delete route.
func newDeleteTestHandler(t *testing.T, id string) *ClipHandler {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := &queue.Job{
		ID:         id,
		Type:       queue.JobTypeClip,
		Name:       "Intro",
		MediaID:    "100",
		MediaTitle: testMovie,
		MediaType:  "movie",
		InputPath:  "/media/movie.mkv",
		OutputPath: "",
		StartTime:  10,
		Duration:   15,
		Quality:    "medium",
		Status:     queue.JobStatusPending,
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
		UpdatedAt:  time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	return NewClipHandler(jobQueue, nil, db, &config.Config{}, nil, "outtake", "test")
}

// deleteClip issues the delete route request for one clip.
//
// Parameters:
//   - t: Test context.
//   - handler: Handler under test.
//   - id: Clip id to delete.
//
// Returns:
//   - status: Response status code.
func deleteClip(t *testing.T, handler *ClipHandler, id string) int {
	t.Helper()

	app := fiber.New()
	app.Delete("/api/clips/:id", handler.Delete)

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodDelete,
		"/api/clips/"+id,
		nil,
	)

	resp, err := app.Test(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp.StatusCode
}

// TestDeleteClipRespondsWithSwappableStatus guards the delete response status.
//
// Htmx skips the swap entirely for 204 and 304, so a 204 makes the card's
// hx-swap="delete" a no-op: the row stays on the page, and an active clip keeps
// polling a route that now 404s. Overriding it with hx-status does not help,
// because the noSwap check returns before that attribute is read.
func TestDeleteClipRespondsWithSwappableStatus(t *testing.T) {
	t.Parallel()

	handler := newDeleteTestHandler(t, "clip-del")

	status := deleteClip(t, handler, "clip-del")

	assert.Equal(t, fiber.StatusOK, status)
	assert.NotContains(t, htmxNoSwap, status, "a no-swap status leaves the card on the page")
}

// TestDeleteClipRemovesStoredRow confirms the 200 did not come at the cost of
// the deletion itself.
func TestDeleteClipRemovesStoredRow(t *testing.T) {
	t.Parallel()

	handler := newDeleteTestHandler(t, "clip-del")

	status := deleteClip(t, handler, "clip-del")
	require.Equal(t, fiber.StatusOK, status)

	found, err := handler.db.GetClip(t.Context(), "clip-del")
	require.ErrorIs(t, err, database.ErrClipNotFound)
	assert.Nil(t, found, "the clip row must be gone")
}

// TestDeleteMissingClipIsNotFound keeps the 404 path intact, since a swap that
// always returns 200 would otherwise hide a failed delete.
func TestDeleteMissingClipIsNotFound(t *testing.T) {
	t.Parallel()

	handler := newDeleteTestHandler(t, "clip-del")

	status := deleteClip(t, handler, "does-not-exist")

	assert.Equal(t, fiber.StatusNotFound, status)
}
