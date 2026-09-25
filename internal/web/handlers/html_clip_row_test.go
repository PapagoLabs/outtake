// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/queue"
)

// newClipRowTestHandler builds an HTMLHandler backed by a throwaway database
// holding one clip.
//
// Parameters:
//   - t: Test context.
//   - id: Clip id to seed.
//   - status: Status to store on the clip.
//   - outputPath: Path of the encoded output, empty when there is none. The
//     card treats a clip as playable only when this file exists on disk.
//
// Returns:
//   - handler: A handler ready to serve the clip row route.
func newClipRowTestHandler(
	t *testing.T,
	id string,
	status queue.JobStatus,
	outputPath string,
) *HTMLHandler {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clips.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	job := &queue.Job{
		ID:         id,
		Type:       queue.JobTypeClip,
		Name:       testClipName,
		MediaID:    "42",
		MediaTitle: testMovie,
		MediaType:  "movie",
		InputPath:  "/media/movie.mkv",
		OutputPath: outputPath,
		StartTime:  1,
		Duration:   5,
		Quality:    defaultQuality,
		Status:     status,
		Progress:   40,
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
		UpdatedAt:  time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := queue.NewQueue(1, noopJobHandler)
	t.Cleanup(jobQueue.Stop)

	return NewHTMLHandler(jobQueue, db, nil, &config.Config{}, "outtake", "test")
}

// realOutputFile writes an empty file the card will treat as an encoded clip.
//
// Parameters:
//   - t: Test context.
//
// Returns:
//   - path: Absolute path to the created file.
func realOutputFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(path, []byte{}, 0o600))

	return path
}

// clipRowRequest issues the poll route request for one clip.
//
// Parameters:
//   - t: Test context.
//   - handler: Handler under test.
//   - id: Clip id to poll.
//
// Returns:
//   - status: Response status code.
//   - body: Response body.
func clipRowRequest(t *testing.T, handler *HTMLHandler, id string) (int, string) {
	t.Helper()

	app := fiber.New()
	app.Get("/clips/:id/row", handler.ClipRow)

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/clips/"+id+"/row",
		nil,
	)

	resp, err := app.Test(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(body)
}

// TestClipRowReturnsOnlyTheStatusRegion is the regression guard for the poll
// target.
//
// The poll swaps its response with outerHTML, so whatever comes back replaces
// the polled element. Returning the whole card would put the edit form inside
// that replacement and discard the user's in-progress changes every two
// seconds.
func TestClipRowReturnsOnlyTheStatusRegion(t *testing.T) {
	t.Parallel()

	handler := newClipRowTestHandler(t, "c1", queue.JobStatusProcessing, "")

	status, body := clipRowRequest(t, handler, "c1")

	require.Equal(t, fiber.StatusOK, status)
	assert.Contains(t, body, `id="clip-c1-status"`)
	assert.NotContains(t, body, `action="/api/clips/c1/update"`,
		"the poll response must not carry the edit form")
	assert.NotContains(t, body, `name="name"`, "the poll response must not carry form controls")
	assert.NotContains(
		t,
		body,
		`name="startTime"`,
		"the poll response must not carry form controls",
	)
}

// TestClipRowSwapsActionButtons confirms the action buttons ride along with the
// poll, so a finished clip stops offering Cancel and starts offering the
// completed actions without a manual reload.
func TestClipRowSwapsActionButtons(t *testing.T) {
	t.Parallel()

	processing := newClipRowTestHandler(t, "c1", queue.JobStatusProcessing, "")
	_, active := clipRowRequest(t, processing, "c1")
	assert.Contains(t, active, "Cancel")
	assert.NotContains(t, active, "Regenerate")

	done := newClipRowTestHandler(t, "c2", queue.JobStatusCompleted, realOutputFile(t))
	_, finished := clipRowRequest(t, done, "c2")
	assert.NotContains(t, finished, "Cancel")
	assert.Contains(t, finished, "Regenerate")
	assert.Contains(t, finished, "Save metadata")
	assert.Contains(t, finished, "/api/clips/c2/download")
}

// TestClipRowRevealsPreviewOnCompletion confirms the polled region still gains
// the player when a clip finishes, so the split did not trade the destructive
// poll for a card that never updates.
func TestClipRowRevealsPreviewOnCompletion(t *testing.T) {
	t.Parallel()

	processing := newClipRowTestHandler(t, "c1", queue.JobStatusProcessing, "")
	_, active := clipRowRequest(t, processing, "c1")
	assert.Contains(t, active, "40%")
	assert.NotContains(t, active, "<video", "no player while the clip is still encoding")

	done := newClipRowTestHandler(t, "c2", queue.JobStatusCompleted, realOutputFile(t))
	_, finished := clipRowRequest(t, done, "c2")
	assert.Contains(t, finished, "Preview")
	assert.NotContains(t, finished, "40%", "the progress bar is gone once complete")
	assert.NotContains(t, finished, `hx-trigger`, "a finished clip stops polling itself")
}

// TestClipRowMissingClipIsNotFound keeps the 404 path intact, since a poll that
// always returned 200 would keep retrying a deleted clip.
func TestClipRowMissingClipIsNotFound(t *testing.T) {
	t.Parallel()

	handler := newClipRowTestHandler(t, "c1", queue.JobStatusProcessing, "")

	status, _ := clipRowRequest(t, handler, "does-not-exist")

	assert.Equal(t, fiber.StatusNotFound, status)
}
