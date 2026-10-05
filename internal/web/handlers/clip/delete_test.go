// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// htmxNoSwap are the statuses that would leave a card on the page after a swap.
var htmxNoSwap = []int{fiber.StatusNoContent, fiber.StatusNotModified}

func newDeleteTestHandler(t *testing.T, id string) *Handler {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := testClipJob(id, clipdom.TypeClip)

	job.Status = clipdom.StatusPending
	job.OutputPath = ""
	job.StartTime = 10 * time.Second
	job.Duration = 15 * time.Second
	job.InputPath = "/media/movie.mkv"
	job.CreatedAt = time.Now().UTC().Truncate(time.Second)
	job.UpdatedAt = job.CreatedAt
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	return New(jobQueue, nil, blob.Paths{}, db, &config.Config{}, nil)
}

func deleteClip(t *testing.T, handler *Handler, id string) int {
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

func TestDeleteClipRespondsWithSwappableStatus(t *testing.T) {
	t.Parallel()

	handler := newDeleteTestHandler(t, "clip-del")

	status := deleteClip(t, handler, "clip-del")

	assert.Equal(t, fiber.StatusOK, status)
	assert.NotContains(t, htmxNoSwap, status, "a no-swap status leaves the card on the page")
}

func TestDeleteClipRemovesStoredRow(t *testing.T) {
	t.Parallel()

	handler := newDeleteTestHandler(t, "clip-del")

	status := deleteClip(t, handler, "clip-del")
	require.Equal(t, fiber.StatusOK, status)

	found, err := handler.db.GetClip(t.Context(), "clip-del")
	require.ErrorIs(t, err, database.ErrClipNotFound)
	assert.Nil(t, found, "the clip row must be gone")
}

func TestDeleteMissingClipIsNotFound(t *testing.T) {
	t.Parallel()

	handler := newDeleteTestHandler(t, "clip-del")

	status := deleteClip(t, handler, "does-not-exist")

	assert.Equal(t, fiber.StatusNotFound, status)
}

func TestDeleteClipRestoresOwnershipWhenTheRowSurvives(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	const id = "delete-ownership"

	job := testClipJob(id, clipdom.TypeClip)

	job.Status = clipdom.StatusPending
	job.OutputPath = ""
	job.CreatedAt = time.Now().UTC().Truncate(time.Second)
	job.UpdatedAt = job.CreatedAt
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)
	jobQueue.Restore(job)

	handler := New(jobQueue, nil, blob.Paths{}, db, &config.Config{}, nil)

	require.NoError(t, db.Close())

	assert.Equal(t, fiber.StatusInternalServerError, deleteClip(t, handler, id),
		"the failure is reported rather than swallowed")

	restored := jobQueue.GetJob(id)
	require.NotNil(t, restored,
		"the row survived, so the job that owns it has to come back")
	assert.Equal(t, clipdom.StatusCancelled, restored.Status,
		"it comes back canceled so the next start does not resubmit a stopped render")
}

func TestDeleteLeavesTheOutputWhenTheRowSurvives(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)

	const id = "delete-keeps-file"

	output := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(output, []byte("clip"), 0o600))

	job := storedDeleteJob(id, output, clipdom.StatusPending)
	require.NoError(t, db.SaveClip(t.Context(), job))

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)
	jobQueue.Restore(job)

	handler := New(jobQueue, store, blob.Paths{}, db, &config.Config{}, nil)

	require.NoError(t, db.Close())

	assert.Equal(t, fiber.StatusInternalServerError, deleteClip(t, handler, id))

	_, statErr := os.Stat(output)
	assert.NoError(t, statErr, "a row that survives keeps the file it names")
}

func TestDeleteRemovesTheOutputAfterTheRow(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const id = "delete-removes-file"

	output := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(output, []byte("clip"), 0o600))

	job := storedDeleteJob(id, output, clipdom.StatusCompleted)
	require.NoError(t, db.SaveClip(t.Context(), job))

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	handler := New(jobQueue, store, blob.Paths{}, db, &config.Config{}, nil)

	require.Equal(t, fiber.StatusOK, deleteClip(t, handler, id))

	_, statErr := os.Stat(output)
	require.ErrorIs(t, statErr, os.ErrNotExist)

	found, err := handler.db.GetClip(t.Context(), id)
	require.ErrorIs(t, err, database.ErrClipNotFound)
	assert.Nil(t, found)
}

func TestDeleteRestoresTheClipWhenTheOutputCannotBeRemoved(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const id = "delete-keeps-row"

	output := filepath.Join(t.TempDir(), "clip-dir")
	require.NoError(t, os.Mkdir(output, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(output, "child"), []byte("x"), 0o600))

	job := storedDeleteJob(id, output, clipdom.StatusCompleted)
	require.NoError(t, db.SaveClip(t.Context(), job))

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	handler := New(jobQueue, store, blob.Paths{}, db, &config.Config{}, nil)

	assert.Equal(t, fiber.StatusInternalServerError, deleteClip(t, handler, id))

	found, err := handler.db.GetClip(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, clipdom.StatusCompleted, found.Status,
		"a finished clip stays finished when its file cannot be removed")

	_, statErr := os.Stat(filepath.Join(output, "child"))
	assert.NoError(t, statErr)
}

func storedDeleteJob(id, output string, status clipdom.Status) *clipdom.Job {
	job := testClipJob(id, clipdom.TypeClip)

	job.Status = status
	job.OutputPath = output
	job.CreatedAt = time.Now().UTC().Truncate(time.Second)
	job.UpdatedAt = job.CreatedAt

	return job
}
