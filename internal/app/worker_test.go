// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/progress"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// testIdleQueue builds an unstarted queue that holds jobs without a worker to
// run them.
//
// Parameters:
//   - t: The test that needs the queue.
//
// Returns:
//   - jobQueue: A queue nothing will ever run.
func testIdleQueue(t *testing.T) *queue.Queue {
	t.Helper()

	jobQueue := queue.NewQueue(0, func(context.Context, *clip.Job) error {
		return context.Canceled
	})
	t.Cleanup(jobQueue.Stop)

	return jobQueue
}

// persistClip writes a clip row the restore tests read back.
//
// Parameters:
//   - t: The test that needs the row.
//   - db: Database the row is written to.
//   - job: The clip to persist.
func persistClip(t *testing.T, db *database.DB, job *clip.Job) {
	t.Helper()

	require.NoError(t, db.SaveClip(t.Context(), job))
}

// testBinding builds an unbound server binding the restore tests set.
//
// Returns:
//   - bind: A binding with no server selected.
func testBinding() *identity.Binding {
	return identity.NewBinding("outtake", "test-client", time.Minute)
}

// plexServerStub serves an empty Plex session list on loopback so a binding's
// session monitor never leaves the process.
//
// Parameters:
//   - t: The test that needs the server.
//
// Returns:
//   - server: The Plex server the binding should be pointed at.
func plexServerStub(t *testing.T) plex.Server {
	t.Helper()

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		_, _ = w.Write([]byte(`{"MediaContainer":{"size":0}}`))
	}))
	t.Cleanup(httpServer.Close)

	parsed, err := url.Parse(httpServer.URL)
	require.NoError(t, err)

	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)

	return plex.Server{
		Name:    "loopback",
		Address: parsed.Hostname(),
		Port:    port,
		Scheme:  "http",
		Token:   "stored-token",
	}
}

// renderableJob builds a clip whose paths are ready for a stubbed render.
//
// Parameters:
//   - t: The test that needs the clip.
//   - dir: Directory the source and output live in.
//
// Returns:
//   - job: A pending clip with a real source file.
func renderableJob(t *testing.T, dir string) *clip.Job {
	t.Helper()

	job := testClipJob("restore-queued")

	job.InputPath = stubInputFile(t, dir, "restored.mkv")
	job.OutputPath = filepath.Join(dir, "restored.mp4")

	return job
}

func TestPersistProgressRecordsThroughSaveProgress(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

	job := testClipJob("progress-recorded")
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := testIdleQueue(t)
	jobQueue.Restore(job)

	persistProgress(t.Context(), job, db, jobQueue)(40)

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, 40, stored.Progress)
}

func TestPersistProgressSkipsACanceledRender(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

	job := testClipJob("progress-canceled")
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := testIdleQueue(t)
	jobQueue.Restore(job)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	persistProgress(ctx, job, db, jobQueue)(40)

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Zero(t, stored.Progress, "a canceled render reports nothing")
}

func TestSaveProgressSkipsACanceledContext(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

	job := testClipJob("save-canceled")
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := testIdleQueue(t)
	jobQueue.Restore(job)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	saveProgress(ctx, job, db, jobQueue, 40)

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Zero(t, stored.Progress)
}

func TestSaveProgressSkipsAJobTheQueueDoesNotHave(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

	job := testClipJob("save-unknown")
	require.NoError(t, db.SaveClip(t.Context(), job))

	saveProgress(t.Context(), job, db, testIdleQueue(t), 40)

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Zero(t, stored.Progress)
}

func TestSaveProgressPersistsTheRecordedProgress(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

	job := testClipJob("save-recorded")
	require.NoError(t, db.SaveClip(t.Context(), job))

	jobQueue := testIdleQueue(t)
	jobQueue.Restore(job)

	saveProgress(t.Context(), job, db, jobQueue, 40)

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, 40, stored.Progress)
}

//nolint:paralleltest // The failure log reads the process-global logger New rewrites.
func TestSaveProgressReportsAWriteFailure(t *testing.T) {
	job := testClipJob("save-failure")

	jobQueue := testIdleQueue(t)
	jobQueue.Restore(job)

	saveProgress(t.Context(), job, closedDatabase(t), jobQueue, 40)

	assert.Equal(t, 40, jobQueue.GetJob(job.ID).Progress,
		"the queue still carries the progress it recorded")
}

//nolint:paralleltest // The queue's submission log reads the process-global logger New rewrites.
func TestStartQueueRestoresPersistedJobs(t *testing.T) {
	db := testDatabase(t)
	persistClip(t, db, testClipJob("restore-queued"))

	jobQueue := startQueue(
		t.Context(),
		workerTestConfig(t),
		db,
		ffmpeg.NewExecFFmpeg(missingBinary(t.TempDir()), missingBinary(t.TempDir())),
		nil,
	)
	t.Cleanup(jobQueue.Stop)

	require.Eventually(t, func() bool {
		restored := jobQueue.GetJob("restore-queued")

		return restored != nil && restored.Status == clip.StatusPending
	}, time.Second, 5*time.Millisecond, "the persisted clip was handed back to the queue")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestStartQueueRunsARestoredRender(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")

	db := testDatabase(t)

	job := renderableJob(t, dir)
	persistClip(t, db, job)

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Put(mock.Anything, job.OutputPath).Return(nil).Once()

	cfg := workerTestConfig(t)

	cfg.NumWorkers = 1

	jobQueue := startQueue(t.Context(), cfg, db,
		ffmpeg.NewExecFFmpeg(stubFFmpeg(t, logPath, ffmpegtest.Stub{}), missingBinary(dir)),
		store,
	)
	t.Cleanup(jobQueue.Stop)

	require.Eventually(t, func() bool {
		rendered := jobQueue.GetJob(job.ID)
		if rendered == nil {
			return false
		}

		return rendered.Status == clip.StatusCompleted
	}, 10*time.Second, 5*time.Millisecond, "the restored clip rendered and settled")

	assert.Equal(t, job.OutputPath, stubOutputArg(stubInvocations(t, logPath)[0]))

	stored, err := db.GetClip(t.Context(), job.ID)
	require.NoError(t, err, "the status callback wrote the settled clip back")
	assert.Equal(t, clip.StatusCompleted, stored.Status)
}

//nolint:paralleltest // The queue's submission log reads the process-global logger New rewrites.
func TestStartQueueReportsAStatusWriteFailure(t *testing.T) {
	jobQueue := startQueue(
		t.Context(),
		workerTestConfig(t),
		closedDatabase(t),
		ffmpeg.NewExecFFmpeg(missingBinary(t.TempDir()), missingBinary(t.TempDir())),
		nil,
	)
	t.Cleanup(jobQueue.Stop)

	assert.NotPanics(t, func() {
		_ = jobQueue.Submit(testClipJob("status-failure"))
	}, "a status the database refuses is logged, not fatal")
}

func TestRestoreJobsReportsAListFailure(t *testing.T) {
	t.Parallel()

	jobQueue := testIdleQueue(t)

	assert.NotPanics(t, func() {
		restoreJobs(t.Context(), closedDatabase(t), jobQueue)
	}, "a list the database refuses is logged, not fatal")

	assert.Empty(t, jobQueue.GetAllJobs())
}

//nolint:paralleltest // The queue's submission log reads the process-global logger New rewrites.
func TestRestoreJobsResubmitsPendingAndProcessingClips(t *testing.T) {
	db := testDatabase(t)

	pending := testClipJob("restore-queued")
	persistClip(t, db, pending)

	processing := testClipJob("restore-processing")

	processing.Status = clip.StatusProcessing
	processing.Error = "encode clip: ffmpeg: exit status 1"
	persistClip(t, db, processing)

	jobQueue := testIdleQueue(t)

	restoreJobs(t.Context(), db, jobQueue)

	for _, id := range []string{"restore-queued", "restore-processing"} {
		restored := jobQueue.GetJob(id)
		require.NotNil(t, restored, "%s was handed back to the queue", id)
		assert.Equal(t, clip.StatusPending, restored.Status,
			"an interrupted clip is picked up again from the start")
		assert.Empty(t, restored.Error, "the previous failure is cleared")
	}
}

func TestRestoreJobsRestoresSettledClips(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)

	settled := testClipJob("restore-settled")

	settled.Status = clip.StatusCompleted
	settled.Progress = 100
	persistClip(t, db, settled)

	failed := testClipJob("restore-failed")

	failed.Status = clip.StatusFailed
	failed.Error = "encode clip: ffmpeg: exit status 1"
	persistClip(t, db, failed)

	jobQueue := testIdleQueue(t)

	restoreJobs(t.Context(), db, jobQueue)

	completed := jobQueue.GetJob(settled.ID)
	require.NotNil(t, completed)
	assert.Equal(t, clip.StatusCompleted, completed.Status, "a finished clip is not run again")

	broken := jobQueue.GetJob(failed.ID)
	require.NotNil(t, broken)
	assert.Equal(t, clip.StatusFailed, broken.Status)
	assert.Equal(t, "encode clip: ffmpeg: exit status 1", broken.Error)
}

func TestRestoreJobsReportsASubmitFailure(t *testing.T) {
	t.Parallel()

	db := testDatabase(t)
	persistClip(t, db, testClipJob("restore-queued"))

	jobQueue := testIdleQueue(t)
	jobQueue.Stop()

	assert.NotPanics(t, func() {
		restoreJobs(t.Context(), db, jobQueue)
	}, "a submit the stopped queue refuses is logged, not fatal")
}

func TestRestoreJobsWithNothingPersisted(t *testing.T) {
	t.Parallel()

	jobQueue := testIdleQueue(t)

	restoreJobs(t.Context(), testDatabase(t), jobQueue)

	assert.Empty(t, jobQueue.GetAllJobs())
}

//nolint:paralleltest // Starting the session monitor reads the process-global logger New rewrites.
func TestRestoreBindingUsesTheConfiguredServer(t *testing.T) {
	server := plexServerStub(t)

	cfg := testAppConfig(t, "")

	cfg.PlexServerURL = "http://" + server.Address + ":" + strconv.Itoa(server.Port)
	cfg.PlexToken = "stored-token"

	bind := testBinding()
	t.Cleanup(bind.Stop)

	restoreBinding(cfg, testDatabase(t), bind)

	bound, ok := bind.Get()
	require.True(t, ok, "the configured server was bound")
	assert.Equal(t, "stored-token", bound.Token)
}

//nolint:paralleltest // Starting the session monitor reads the process-global logger New rewrites.
func TestRestoreBindingFallsBackToTheStoredServer(t *testing.T) {
	db := testDatabase(t)
	require.NoError(t, db.SaveSelectedServer(t.Context(), plexServerStub(t)))

	bind := testBinding()
	t.Cleanup(bind.Stop)

	restoreBinding(testAppConfig(t, ""), db, bind)

	bound, ok := bind.Get()
	require.True(t, ok, "the stored server was bound")
	assert.Equal(t, "stored-token", bound.Token)
}

func TestRestoreBindingLeavesTheBindingUnsetWithoutAServer(t *testing.T) {
	t.Parallel()

	bind := testBinding()
	t.Cleanup(bind.Stop)

	restoreBinding(testAppConfig(t, ""), testDatabase(t), bind)

	_, ok := bind.Get()
	assert.False(t, ok, "nothing was selected, so nothing is bound")
}

func TestRestoreBindingReportsAReadFailure(t *testing.T) {
	t.Parallel()

	bind := testBinding()
	t.Cleanup(bind.Stop)

	assert.NotPanics(t, func() {
		restoreBinding(testAppConfig(t, ""), closedDatabase(t), bind)
	}, "a read the database refuses is logged, not fatal")

	_, ok := bind.Get()
	assert.False(t, ok)
}

func TestProgressContextCarriesTheJobCallback(t *testing.T) {
	t.Parallel()

	var seen int

	ctx := progress.WithProgress(t.Context(), func(percent int) {
		seen = percent
	})

	progress.From(ctx)(40)

	assert.Equal(t, 40, seen)
}

// workerTestConfig builds a config whose queue holds jobs without a worker.
//
// Parameters:
//   - t: The test that needs the config.
//
// Returns:
//   - cfg: A configuration with no queue workers.
func workerTestConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg := testAppConfig(t, "")

	cfg.NumWorkers = 0

	return cfg
}
