// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/app"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// clipListing is the payload the clips collection answers with.
type clipListing struct {
	Clips []api.ClipResponse `json:"clips"`
}

// clipSession holds the cookies and CSRF token a mutating request has to carry.
//
// Parameters:
//   - cookies: Cookies the login page set.
//   - token: CSRF token to send, empty to leave the token out of the request.
type clipSession struct {
	cookies []*http.Cookie
	token   string
}

// apiAnswer is what a route answered with once its body has been drained.
type apiAnswer struct {
	status      int
	contentType string
	payload     []byte
}

// appConfig returns a configuration rooted in the test's temp directory.
//
// Parameters:
//   - t: The test that needs the configuration.
//
// Returns:
//   - cfg: The configuration every application under test is built from.
func appConfig(t *testing.T) *config.Config {
	t.Helper()

	dir := t.TempDir()

	return &config.Config{
		ListenAddr:      "127.0.0.1:0",
		DatabasePath:    filepath.Join(dir, "outtake.db"),
		DatabaseBackend: "sqlite",
		StoragePath:     filepath.Join(dir, "output"),
		StorageBackend:  "filesystem",
		FFmpegPath:      "outtake-integration-absent-ffmpeg",
		FFprobePath:     "outtake-integration-absent-ffmpeg",
		LogLevel:        "error",
		Env:             "e2e",
		SessionPoll:     60 * time.Second,

		// The HTTP tests read a created clip back while it is still pending,
		// so they run without a render worker. The lifecycle tests turn one
		// back on to watch a render through.
		NumWorkers:            0,
		MaxConcurrentPreviews: 1,
		MaxClipDur:            600 * time.Second,
		PlexClientID:          "integration-client",

		// httptest requests name example.com as their host.
		AllowedHosts: "example.com",
	}
}

// renderingApp returns a running application whose queue renders work.
//
// Parameters:
//   - t: The test that needs the application.
//   - cfg: Configuration naming the database and storage the app should use.
//
// Returns:
//   - application: The wired application, closed when the test finishes.
func renderingApp(t *testing.T, cfg *config.Config) *app.App {
	t.Helper()

	cfg.NumWorkers = 1

	application, err := app.New(cfg)
	require.NoError(t, err)

	t.Cleanup(application.Close)

	return application
}

// startedApp returns a running application whose queue accepts work but renders none.
//
// Parameters:
//   - t: The test that needs the application.
//
// Returns:
//   - application: The wired application, closed when the test finishes.
//   - cfg: The configuration it was built from.
func startedApp(t *testing.T) (*app.App, *config.Config) {
	t.Helper()

	cfg := appConfig(t)

	application, err := app.New(cfg)
	require.NoError(t, err)

	t.Cleanup(application.Close)

	return application, cfg
}

// storedDatabase returns a second handle onto the application's database file.
//
// Parameters:
//   - t: The test that needs the handle.
//   - cfg: Configuration naming the database file.
//
// Returns:
//   - db: An independent handle onto the same SQLite file.
func storedDatabase(t *testing.T, cfg *config.Config) *database.DB {
	t.Helper()

	db, err := database.New(cfg.DatabasePath)
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// e2eMediaFile writes the media file a clip is cut from.
//
// Parameters:
//   - t: The test that needs the file.
//
// Returns:
//   - path: The file path, which is also the media id under the e2e environment.
func e2eMediaFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("not really a video"), 0o644))

	return path
}

// storedClip returns a clip row for the database side of a lifecycle test.
//
// Parameters:
//   - id: Clip identifier.
//   - inputPath: Source the clip is cut from.
//   - status: Status the row starts at.
//
// Returns:
//   - job: A clip with the timings and destination the app would have written.
func storedClip(id, inputPath string, status clip.Status) *clip.Job {
	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

	return &clip.Job{
		ID:         id,
		Type:       clip.TypeClip,
		Name:       "Opening Beat",
		MediaID:    inputPath,
		MediaTitle: "Integration Movie",
		MediaType:  clip.DefaultMediaType,

		Quality:   "",
		StartTime: 2 * time.Second,
		Duration:  8 - 2,

		CreatedAt: now,
		UpdatedAt: now, InputPath: inputPath,
		// Every clip the create route stores has a destination.
		OutputPath: filepath.Join(filepath.Dir(inputPath), id+".mp4"),

		Status: status,
	}
}

// openSession fetches the login page and takes its CSRF token and cookies.
//
// Parameters:
//   - t: The test that is opening the session.
//   - application: Application under test.
//
// Returns:
//   - session: The cookies and token a mutating request has to send.
func openSession(t *testing.T, application *app.App) clipSession {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", http.NoBody)

	response, err := application.Test(request)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, response.Body.Close())
	}()

	require.Equal(t, http.StatusOK, response.StatusCode)

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	_, after, found := strings.Cut(string(body), `<meta name="csrf-token" content="`)
	require.True(t, found, "the login page carries the CSRF token the form posts need")

	token, _, found := strings.Cut(after, `"`)
	require.True(t, found, "the CSRF token is a complete attribute value")
	require.NotEmpty(t, token)

	return clipSession{cookies: response.Cookies(), token: token}
}

// call issues a JSON request against the application and drains its answer.
//
// Parameters:
//   - t: The test that is issuing the request.
//   - application: Application under test.
//   - session: Session the request is issued under.
//   - method: HTTP method to use.
//   - path: Route to call.
//   - body: Payload to encode, or nil for an empty body.
//
// Returns:
//   - answer: The status, content type, and body the route answered with.
func call(
	t *testing.T,
	application *app.App,
	session clipSession,
	method, path string,
	body any,
) apiAnswer {
	t.Helper()

	var reader io.Reader = http.NoBody

	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)

		reader = bytes.NewReader(encoded)
	}

	request := httptest.NewRequestWithContext(t.Context(), method, path, reader)
	request.Header.Set("Content-Type", "application/json")

	if session.token != "" {
		request.Header.Set("X-Csrf-Token", session.token)
	}

	for _, cookie := range session.cookies {
		request.AddCookie(cookie)
	}

	response, err := application.Test(request)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, response.Body.Close())
	}()

	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return apiAnswer{
		status:      response.StatusCode,
		contentType: response.Header.Get("Content-Type"),
		payload:     payload,
	}
}

// decodeInto unmarshals an answer body onto a payload pointer.
//
// Parameters:
//   - t: The test that is decoding.
//   - answer: The answer to decode.
//   - out: Destination the body is unmarshalled onto.
func decodeInto(t *testing.T, answer apiAnswer, out any) {
	t.Helper()

	require.NoError(t, json.Unmarshal(answer.payload, out))
}

// createClip posts a create clip request and returns the clip the app answered with.
//
// Parameters:
//   - t: The test that is creating the clip.
//   - application: Application under test.
//   - session: Session the post is issued under.
//   - mediaID: Media item the clip is cut from.
//
// Returns:
//   - created: The payload the create route answered with.
func createClip(
	t *testing.T,
	application *app.App,
	session clipSession,
	mediaID string,
) api.ClipResponse {
	t.Helper()

	answer := call(t, application, session, http.MethodPost, "/api/clips", api.ClipRequest{
		Name:       "Opening Beat",
		MediaID:    mediaID,
		MediaTitle: "Integration Movie",
		MediaType:  clip.DefaultMediaType,
		StartTime:  2,
		Duration:   8 - 2,
		Quality:    "",
		ClipType:   string(clip.TypeClip),
	})

	require.Equal(t, http.StatusCreated, answer.status)

	var created api.ClipResponse

	decodeInto(t, answer, &created)

	return created
}

// readClip reads one clip back through the status route.
//
// Parameters:
//   - t: The test that is reading the clip.
//   - application: Application under test.
//   - id: Clip identifier.
//
// Returns:
//   - status: The HTTP status the route answered with.
//   - payload: The decoded payload, zero when the route answered with an error.
func readClip(t *testing.T, application *app.App, id string) (int, api.ClipResponse) {
	t.Helper()

	answer := call(
		t, application, clipSession{}, http.MethodGet, "/api/clips"+"/"+id+"/status", nil,
	)
	if answer.status != http.StatusOK {
		return answer.status, api.ClipResponse{}
	}

	payload := api.ClipResponse{}

	decodeInto(t, answer, &payload)

	return answer.status, payload
}

// awaitSettled polls the status route until the clip reports a settled status.
//
// Parameters:
//   - t: The test that is waiting.
//   - application: Application under test.
//   - id: Clip identifier.
//
// Returns:
//   - settled: The last clip payload read.
func awaitSettled(t *testing.T, application *app.App, id string) api.ClipResponse {
	t.Helper()

	settled := api.ClipResponse{}

	require.Eventually(t, func() bool {
		_, payload := readClip(t, application, id)
		if payload.Status == "" {
			return false
		}

		settled = payload

		switch payload.Status {
		case clip.StatusCompleted, clip.StatusFailed:
			return true
		default:
			return false
		}
	}, 15*time.Second, 5*time.Millisecond, "the clip settled")

	return settled
}

// listClips reads the clips collection.
//
// Parameters:
//   - t: The test that is listing the clips.
//   - application: Application under test.
//
// Returns:
//   - status: The HTTP status the route answered with.
//   - payload: The decoded listing.
func listClips(t *testing.T, application *app.App) (int, clipListing) {
	t.Helper()

	answer := call(t, application, clipSession{}, http.MethodGet, "/api/clips", nil)
	if answer.status != http.StatusOK {
		return answer.status, clipListing{}
	}

	payload := clipListing{}

	decodeInto(t, answer, &payload)

	return answer.status, payload
}

// storedRow reads one clip out of the clips table, retrying while SQLite
// reports the writer's lock.
//
// Parameters:
//   - t: The test that is reading the clip.
//   - db: Independent handle onto the application's database file.
//   - id: Clip identifier.
//
// Returns:
//   - stored: The clip as the table holds it.
func storedRow(t *testing.T, db *database.DB, id string) *clip.Job {
	t.Helper()

	var stored *clip.Job

	require.Eventually(t, func() bool {
		job, err := db.GetClip(t.Context(), id)
		if err != nil {
			return false
		}

		stored = job

		return true
	}, 15*time.Second, 5*time.Millisecond, "the clips table could be read")

	return stored
}

// awaitStoredOutcome waits for a stored clip to carry a settled status.
//
// Parameters:
//   - t: The test that is waiting.
//   - db: Independent handle onto the application's database file.
//   - id: Clip identifier.
//
// Returns:
//   - stored: The settled clip as the table holds it.
func awaitStoredOutcome(t *testing.T, db *database.DB, id string) *clip.Job {
	t.Helper()

	var stored *clip.Job

	require.Eventually(t, func() bool {
		job, err := db.GetClip(t.Context(), id)
		if err != nil {
			return false
		}

		switch job.Status {
		case clip.StatusCompleted, clip.StatusFailed, clip.StatusCancelled:
			stored = job

			return true
		default:
			return false
		}
	}, 15*time.Second, 5*time.Millisecond, "the settled outcome reached the clips table")

	return stored
}

// storedRowIsGone waits for a clip to leave the clips table.
//
// Parameters:
//   - t: The test that is waiting.
//   - db: Independent handle onto the application's database file.
//   - id: Clip identifier.
func storedRowIsGone(t *testing.T, db *database.DB, id string) {
	t.Helper()

	require.Eventually(t, func() bool {
		_, err := db.GetClip(t.Context(), id)

		return errors.Is(err, database.ErrClipNotFound)
	}, 15*time.Second, 5*time.Millisecond, "the clips table no longer holds the clip")
}

//nolint:paralleltest // app.New rewrites process-wide logging globals, so these tests must not overlap.
func TestIntegration_HealthzAnswersWithoutTouchingAnExternalService(t *testing.T) {
	application, _ := startedApp(t)

	answer := call(t, application, clipSession{}, http.MethodGet, "/api/healthz", nil)

	assert.Equal(t, http.StatusOK, answer.status)
	assert.Contains(t, answer.contentType, "application/json")
}

//nolint:paralleltest // app.New rewrites process-wide logging globals, so these tests must not overlap.
func TestIntegration_CreatedClipReachesStorageAndTheStatusRoute(t *testing.T) {
	application, cfg := startedApp(t)
	db := storedDatabase(t, cfg)

	session := openSession(t, application)
	mediaID := e2eMediaFile(t)
	created := createClip(t, application, session, mediaID)

	require.NotEmpty(t, created.ID)
	assert.Equal(t, "Opening Beat", created.Name)
	assert.Equal(t, "Integration Movie", created.MediaTitle)
	assert.Equal(t, clip.TypeClip, created.ClipType)
	assert.Equal(t, clip.StatusPending, created.Status,
		"a clip is accepted pending, before anything has rendered it")
	assert.Empty(t, created.InputPath, "the source path is never published")
	assert.Empty(t, created.OutputPath)

	stored := storedRow(t, db, created.ID)
	assert.Equal(t, "Opening Beat", stored.Name)
	assert.Equal(t, mediaID, stored.InputPath)
	assert.Equal(t, "Integration Movie", stored.MediaTitle)
	assert.Equal(t, 2*time.Second, stored.StartTime)
	assert.Equal(t, (8-2)*time.Second, stored.Duration)
	assert.Equal(t, cfg.StoragePath, filepath.Dir(filepath.Dir(stored.OutputPath)),
		"the destination sits under the configured storage layout")

	status, read := readClip(t, application, created.ID)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, created.ID, read.ID)
	assert.Equal(t, clip.StatusPending, read.Status)

	listed, payload := listClips(t, application)
	assert.Equal(t, http.StatusOK, listed)
	require.Len(t, payload.Clips, 1)
	assert.Equal(t, created.ID, payload.Clips[0].ID)
}

//nolint:paralleltest // app.New rewrites process-wide logging globals, so these tests must not overlap.
func TestIntegration_RenderOutcomeReachesTheDatabase(t *testing.T) {
	cfg := appConfig(t)
	db := storedDatabase(t, cfg)

	mediaID := e2eMediaFile(t)

	// A stored pending clip is what the application finds when it starts, so
	// the render, the queue transition, and the write-back are all exercised
	// through the restore path.
	require.NoError(
		t,
		db.SaveClip(t.Context(), storedClip("restored-clip", mediaID, clip.StatusPending)),
	)

	application := renderingApp(t, cfg)

	settled := awaitSettled(t, application, "restored-clip")

	// The render cannot succeed on a host without the binary the configuration
	// names, so what must hold either way is that the outcome reached storage
	// and that either outcome is described consistently.
	switch settled.Status {
	case clip.StatusCompleted:
		assert.Equal(t, 100, settled.Progress)
		assert.Empty(t, settled.Error)
	case clip.StatusFailed:
		assert.NotEmpty(t, settled.Error, "a failure carries the reason it failed")
		assert.Contains(t, settled.Error, "outtake-integration-absent-ffmpeg")
	default:
		require.Fail(t, "the clip never settled", "status was "+string(settled.Status))
	}

	// The status route is answered from the queue, so it can report a settled
	// status before the status callback has committed the row. Waiting on the
	// row matching is what makes the write-back observable.
	stored := awaitStoredOutcome(t, db, "restored-clip")
	assert.Equal(t, settled.Status, stored.Status,
		"the queue's status callback wrote the outcome through")
	assert.Equal(t, settled.Error, stored.Error)
}

//nolint:paralleltest // app.New rewrites process-wide logging globals, so these tests must not overlap.
func TestIntegration_DeletedClipLeavesTheDatabaseAndTheListing(t *testing.T) {
	application, cfg := startedApp(t)
	db := storedDatabase(t, cfg)

	session := openSession(t, application)
	created := createClip(t, application, session, e2eMediaFile(t))

	del := call(t, application, session, http.MethodDelete, "/api/clips"+"/"+created.ID, nil)
	assert.Equal(t, http.StatusOK, del.status)

	storedRowIsGone(t, db, created.ID)

	status, _ := readClip(t, application, created.ID)
	assert.Equal(t, http.StatusNotFound, status)

	again := call(t, application, session, http.MethodDelete, "/api/clips"+"/"+created.ID, nil)
	assert.Equal(
		t,
		http.StatusNotFound,
		again.status,
		"deleting an unknown clip is a not found",
	)

	_, payload := listClips(t, application)
	assert.Empty(t, payload.Clips, "the deleted clip is gone from the listing")
}

//nolint:paralleltest // app.New rewrites process-wide logging globals, so these tests must not overlap.
func TestIntegration_StoredClipsSurviveARestart(t *testing.T) {
	cfg := appConfig(t)
	db := storedDatabase(t, cfg)

	settledJob := storedClip("kept-clip", e2eMediaFile(t), clip.StatusCompleted)

	settledJob.Progress = 100

	pendingJob := storedClip("requeued-clip", e2eMediaFile(t), clip.StatusPending)

	pendingJob.CreatedAt = settledJob.CreatedAt.Add(-time.Hour)

	require.NoError(t, db.SaveClip(t.Context(), settledJob))
	require.NoError(t, db.SaveClip(t.Context(), pendingJob))

	application := renderingApp(t, cfg)

	_, payload := listClips(t, application)
	require.Len(t, payload.Clips, 2, "a stored clip is listed again after a start")

	byID := make(map[string]api.ClipResponse, len(payload.Clips))
	for _, listed := range payload.Clips {
		byID[listed.ID] = listed
	}

	assert.Equal(t, clip.StatusCompleted, byID[settledJob.ID].Status,
		"a settled clip is restored as it was, not re-rendered")
	assert.Equal(t, 100, byID[settledJob.ID].Progress)

	requeued := awaitSettled(t, application, pendingJob.ID)
	assert.Equal(t, clip.StatusFailed, requeued.Status,
		"a stored pending clip is queued for rendering again and reaches its own outcome")
	assert.Contains(t, requeued.Error, "outtake-integration-absent-ffmpeg")

	_, afterRequeue := readClip(t, application, settledJob.ID)
	assert.Equal(t, clip.StatusCompleted, afterRequeue.Status,
		"rendering the pending clip left the settled one alone")
}

//nolint:paralleltest // app.New rewrites process-wide logging globals, so these tests must not overlap.
func TestIntegration_CreateRefusesAClipThatCannotBeValidated(t *testing.T) {
	application, _ := startedApp(t)

	session := openSession(t, application)

	answer := call(t, application, session, http.MethodPost, "/api/clips", api.ClipRequest{
		Name:       "Opening Beat",
		MediaID:    filepath.Join(t.TempDir(), "absent.mkv"),
		MediaTitle: "Integration Movie",
		MediaType:  clip.DefaultMediaType,
		StartTime:  2,
		Duration:   8 - 2,
		Quality:    "",
		ClipType:   string(clip.TypeClip),
	})

	assert.Equal(t, http.StatusBadRequest, answer.status)

	var failure api.ErrorResponse

	decodeInto(t, answer, &failure)
	assert.Equal(t, api.MediaPathUnresolved, failure.Error)

	_, payload := listClips(t, application)
	assert.Empty(t, payload.Clips, "a refused create stored nothing")
}

//nolint:paralleltest // app.New rewrites process-wide logging globals, so these tests must not overlap.
func TestIntegration_MutatingRoutesRefuseARequestWithoutTheToken(t *testing.T) {
	application, _ := startedApp(t)

	session := openSession(t, application)

	answer := call(
		t,
		application,
		clipSession{cookies: session.cookies},
		http.MethodPost,
		"/api/clips",
		api.ClipRequest{
			Name:       "Opening Beat",
			MediaID:    e2eMediaFile(t),
			MediaTitle: "Integration Movie",
			StartTime:  2,
			Duration:   8 - 2,
		},
	)

	assert.Equal(t, http.StatusForbidden, answer.status)
}
