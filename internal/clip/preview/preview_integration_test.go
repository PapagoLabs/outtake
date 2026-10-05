// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/preview"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/progress"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

// progressStep is one progress report a render makes, acknowledged once applied.
type progressStep struct {
	percent int
	applied chan struct{}
}

// batchNames is the fixture set of preview identifiers a batch admission fills.
var batchNames = []string{"preview-a", "preview-b", "preview-c", "preview-d"}

// previewService returns a preview service over a real filesystem backend.
//
// Parameters:
//   - t: The test that needs the service.
//
// Returns:
//   - service: The preview service.
//   - store: The filesystem backend renders publish through.
func previewService(t *testing.T) (*preview.Service, *blob.Storage) {
	t.Helper()

	paths := blob.NewPaths(filepath.Join(t.TempDir(), "output"))

	store, err := blob.NewStorage(paths)
	require.NoError(t, err)

	service := preview.New(1, store, paths, ffmpeg.NewExecFFmpeg("", ""))

	return service, store
}

// progressRenderer returns a render that reports whatever steps it is fed.
//
// Parameters:
//   - t: The test that owns the render.
//
// Returns:
//   - fn: The render the service runs in the background.
//   - steps: Channel the test sends each report on.
//   - ran: Closed once the render returns.
func progressRenderer(
	t *testing.T,
) (func(context.Context) error, chan progressStep, chan struct{}) {
	t.Helper()

	steps := make(chan progressStep)
	ran := make(chan struct{})

	fn := func(ctx context.Context) error {
		report := progress.From(ctx)

		for step := range steps {
			report(step.percent)
			close(step.applied)
		}

		close(ran)

		return nil
	}

	return fn, steps, ran
}

// sendProgress feeds one report to a render and waits for it to be applied.
//
// Parameters:
//   - t: The test that is reporting progress.
//   - steps: Channel the render reads its reports from.
//   - percent: Percentage to report.
func sendProgress(t *testing.T, steps chan<- progressStep, percent int) {
	t.Helper()

	applied := make(chan struct{})
	steps <- progressStep{percent: percent, applied: applied}

	select {
	case <-applied:
	case <-time.After(5 * time.Second):
		require.Fail(t, "the render never applied the reported progress")
	}
}

// awaitProgress waits for a preview to report the wanted percentage.
//
// Parameters:
//   - t: The test that is waiting.
//   - service: Service holding the render.
//   - previewID: Preview to read.
//
// Returns:
//   - view: The snapshot the preview reported.
func awaitProgress(
	t *testing.T,
	service *preview.Service,
	previewID string,
	percent int,
) preview.View {
	t.Helper()

	require.Eventually(t, func() bool {
		view, ok := service.Status(previewID)

		return ok && view.Progress == percent
	}, 5*time.Second, time.Millisecond, "the preview reported "+strconv.Itoa(percent)+" percent")

	view, ok := service.Status(previewID)
	require.True(t, ok)

	return view
}

// awaitTerminal waits for a preview to reach a settled status.
//
// Parameters:
//   - t: The test that is waiting.
//   - service: Service holding the render.
//   - previewID: Preview to read.
//   - want: Status to wait for.
//
// Returns:
//   - view: The settled snapshot.
func awaitTerminal(
	t *testing.T,
	service *preview.Service,
	previewID string,
	want clip.Status,
) preview.View {
	t.Helper()

	require.Eventually(t, func() bool {
		view, ok := service.Status(previewID)

		return ok && view.Status == want
	}, 5*time.Second, time.Millisecond, "the preview reached "+string(want))

	view, ok := service.Status(previewID)
	require.True(t, ok)

	return view
}

// blockedRender returns a render that parks until it is released or canceled.
//
// Parameters:
//   - t: The test that owns the render.
//
// Returns:
//   - fn: The blocking render.
//   - release: Releases every parked render. It is safe to call more than once.
//   - entered: Closed once a render has entered the handler.
func blockedRender(t *testing.T) (func(context.Context) error, func(), <-chan struct{}) {
	t.Helper()

	release := make(chan struct{})
	entered := make(chan struct{})

	var releaseOnce sync.Once

	var enteredOnce sync.Once

	unblock := func() {
		releaseOnce.Do(func() { close(release) })
	}

	t.Cleanup(unblock)

	fn := func(ctx context.Context) error {
		enteredOnce.Do(func() { close(entered) })

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}

	return fn, unblock, entered
}

func TestIntegration_RenderRecordsOnlyForwardProgress(t *testing.T) {
	t.Parallel()

	service, _ := previewService(t)

	fn, steps, ran := progressRenderer(t)
	require.Equal(t, preview.AdmittedRender, service.Submit(t.Context(), "preview-1", fn))

	entry, ok := service.Status("preview-1")
	require.True(t, ok)
	assert.Equal(t, "preview-1", entry.ID)

	sendProgress(t, steps, 40)
	assert.Equal(t, 40, awaitProgress(t, service, "preview-1", 40).Progress)

	sendProgress(t, steps, 10)

	view, ok := service.Status("preview-1")
	require.True(t, ok)
	assert.Equal(t, 40, view.Progress, "a report that goes backwards is ignored")

	sendProgress(t, steps, 40)

	view, ok = service.Status("preview-1")
	require.True(t, ok)
	assert.Equal(t, 40, view.Progress, "a repeated report changes nothing")

	sendProgress(t, steps, 250)
	assert.Equal(t, 100, awaitProgress(t, service, "preview-1", 100).Progress,
		"a report past the end clamps to a full render")

	close(steps)
	<-ran

	settled := awaitTerminal(t, service, "preview-1", clip.StatusCompleted)
	assert.Equal(t, 100, settled.Progress)
	assert.True(t, settled.Done())
	assert.Empty(t, settled.Error)
}

func TestIntegration_RenderThatFailsRecordsTheError(t *testing.T) {
	t.Parallel()

	service, _ := previewService(t)

	failed := service.Submit(t.Context(), "preview-1", func(context.Context) error {
		return os.ErrPermission
	})
	require.Equal(t, preview.AdmittedRender, failed)

	settled := awaitTerminal(t, service, "preview-1", clip.StatusFailed)
	assert.Contains(t, settled.Error, os.ErrPermission.Error())
	assert.True(t, settled.Done())
}

func TestIntegration_AdmissionRefusesOnceEverySlotIsOutstanding(t *testing.T) {
	t.Parallel()

	service, _ := previewService(t)

	fn, release, entered := blockedRender(t)

	for _, id := range batchNames {
		require.Equal(t, preview.AdmittedRender, service.Submit(t.Context(), id, fn))
	}

	<-entered

	refused := service.Submit(t.Context(), "preview-refused", fn)
	assert.Equal(t, preview.RefusedFull, refused,
		"four outstanding previews fill the single slot")

	_, ok := service.Status("preview-refused")
	assert.False(t, ok, "a refused preview is never registered")

	release()

	for _, id := range batchNames {
		awaitTerminal(t, service, id, clip.StatusCompleted)
	}

	assert.Equal(t, preview.AdmittedRender, service.Submit(t.Context(), "preview-2", fn),
		"a slot frees up as soon as a render finishes")
}

func TestIntegration_DuplicateSubmissionJoinsTheRunningRender(t *testing.T) {
	t.Parallel()

	service, _ := previewService(t)

	fn, release, entered := blockedRender(t)

	var calls atomic.Int64

	counted := func(ctx context.Context) error {
		calls.Add(1)

		return fn(ctx)
	}

	require.Equal(t, preview.AdmittedRender, service.Submit(t.Context(), "preview-1", counted))
	<-entered

	assert.Equal(t, preview.AdmittedExisting, service.Submit(t.Context(), "preview-1", counted),
		"the same preview arriving twice is joined, not rerun")
	assert.Equal(t, int64(1), calls.Load())

	release()
	awaitTerminal(t, service, "preview-1", clip.StatusCompleted)

	assert.Equal(t, preview.AdmittedRender, service.Submit(t.Context(), "preview-1", fn),
		"a settled entry is retired so the id can be rendered again")
}

func TestIntegration_CancelStopsARunningPreview(t *testing.T) {
	t.Parallel()

	service, _ := previewService(t)

	fn, _, entered := blockedRender(t)
	require.Equal(t, preview.AdmittedRender, service.Submit(t.Context(), "preview-1", fn))
	<-entered

	assert.True(t, service.Cancel("preview-1"))

	canceled := awaitTerminal(t, service, "preview-1", clip.StatusCancelled)
	assert.True(t, canceled.Done())
	assert.Empty(t, canceled.Error, "a cancellation is not a failure")

	assert.False(t, service.Cancel("preview-1"), "a settled preview is not cancellable")
	assert.False(t, service.Cancel("preview-2"))
}

func TestIntegration_AcquireRefusesOnceEverySlotIsHeld(t *testing.T) {
	t.Parallel()

	service, _ := previewService(t)

	assert.Equal(t, 1, service.Slots())

	release, err := service.Acquire(t.Context(), 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, release)

	_, err = service.Acquire(t.Context(), 20*time.Millisecond)
	require.ErrorIs(t, err, preview.ErrBusy)

	release()

	second, err := service.Acquire(t.Context(), 5*time.Second)
	require.NoError(t, err)

	second()
}

func TestIntegration_RenderIntoSkipsAPreviewThatIsAlreadyPublished(t *testing.T) {
	t.Parallel()

	service, store := previewService(t)

	assert.False(t, service.Published("preview-1"))

	output := service.OutputPath("preview-1")
	require.Equal(t, store.PreviewPath("preview-1"), output)

	require.NoError(t, os.WriteFile(output, []byte("already rendered"), 0o644))
	assert.True(t, store.FileExists(output))

	require.NoError(
		t,
		service.RenderInto(
			t.Context(),
			"preview-1",
			"/media/source.mkv",
			clip.Request{},
			false,
		),
	)

	assert.True(t, store.FileExists(output), "the cached preview was left alone")
}

func TestIntegration_DiscardedRendersLeaveNothingBehind(t *testing.T) {
	t.Parallel()

	_, store := previewService(t)

	staged := store.PreviewPath("staged-uuid")
	require.NoError(t, os.WriteFile(staged, []byte("half rendered"), 0o644))

	preview.DiscardStaged(staged)
	assert.False(t, store.FileExists(staged))

	preview.DiscardStaged(staged)

	published := store.PreviewPath("published-id")
	require.NoError(t, os.WriteFile(published, []byte("upload failed"), 0o644))

	require.NoError(t, preview.DiscardPublished(store, published))
	assert.False(t, store.FileExists(published))

	require.NoError(t, preview.DiscardPublished(store, published))
}

func TestIntegration_RememberMakesAPublishedPreviewTerminal(t *testing.T) {
	t.Parallel()

	service, _ := previewService(t)

	_, ok := service.Status("preview-1")
	assert.False(t, ok, "an id nothing knows about has no status")

	service.Remember("preview-1")

	view, ok := service.Status("preview-1")
	require.True(t, ok)
	assert.Equal(t, clip.StatusCompleted, view.Status)
	assert.Equal(t, 100, view.Progress)
	assert.True(t, view.Done())

	fn, release, _ := blockedRender(t)
	t.Cleanup(release)

	assert.Equal(t, preview.AdmittedRender, service.Submit(t.Context(), "preview-1", fn),
		"a remembered preview does not block the id from being rendered again")
}
