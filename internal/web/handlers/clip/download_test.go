// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/blob"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// renderedOutput writes a rendered file and returns the path a clip records as
// its output.
//
// Parameters:
//   - t: The test the output belongs to.
//   - name: Filename to write.
//
// Returns:
//   - path: Path of the written file.
func renderedOutput(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte("encoded"), 0o600))

	return path
}

func newDownloadTestHandler(t *testing.T, job *clipdom.Job) *Handler {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	return New(jobQueue, store, store.Paths, db, &config.Config{}, nil)
}

func downloadClip(t *testing.T, handler *Handler, id string) *http.Response {
	t.Helper()

	app := fiber.New()
	app.Get("/api/clips/:id/download", handler.Download)

	resp, err := app.Test(httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/clips/"+id+"/download", nil,
	))
	require.NoError(t, err)

	return resp
}

func downloadStatus(t *testing.T, handler *Handler, id string) int {
	t.Helper()

	resp := downloadClip(t, handler, id)

	defer closeBody(t, resp)

	return resp.StatusCode
}

func TestDownloadOffersTheFileUnderTheClipName(t *testing.T) {
	t.Parallel()

	job := testClipJob("c1", clipdom.TypeGIF)

	job.Status = clipdom.StatusCompleted
	job.OutputPath = renderedOutput(t, "clipdom.gif")

	handler := newDownloadTestHandler(t, job)

	resp := downloadClip(t, handler, job.ID)
	defer closeBody(t, resp)

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get(fiber.HeaderContentDisposition), "Intro.gif",
		"the file is offered as a GIF under the clip's own name")
}

func TestDownloadRefusesAClipThatIsNotFinished(t *testing.T) {
	t.Parallel()

	job := testClipJob("c1", clipdom.TypeClip)

	job.Status = clipdom.StatusProcessing
	job.OutputPath = renderedOutput(t, "clipdom.mp4")

	handler := newDownloadTestHandler(t, job)

	assert.Equal(t, fiber.StatusConflict, downloadStatus(t, handler, job.ID))
}

func TestDownloadReportsAMissingOutput(t *testing.T) {
	t.Parallel()

	job := testClipJob("c1", clipdom.TypeClip)

	job.Status = clipdom.StatusCompleted
	job.OutputPath = filepath.Join(t.TempDir(), "never-rendered.mp4")

	handler := newDownloadTestHandler(t, job)

	assert.Equal(t, fiber.StatusNotFound, downloadStatus(t, handler, job.ID))
}

func TestDownloadMissingClipIsNotFound(t *testing.T) {
	t.Parallel()

	job := testClipJob("c1", clipdom.TypeClip)

	job.Status = clipdom.StatusCompleted

	handler := newDownloadTestHandler(t, job)

	assert.Equal(t, fiber.StatusNotFound, downloadStatus(t, handler, "absent"))
}
