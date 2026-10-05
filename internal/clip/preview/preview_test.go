// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

func newPendingJob(name string) *entry {
	return &entry{
		view:    View{ID: name, Status: "pending"},
		cancel:  func() {},
		created: time.Now(),
		updated: time.Now(),
	}
}

func newServiceFixture(t *testing.T, limit int) (*Service, *blob.Storage) {
	t.Helper()

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	return New(limit, store, store.Paths, stagingFFmpeg(t, false)), store
}

func awaitTerminal(t *testing.T, registry *registry, previewID string) View {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		view, ok := registry.resolve(previewID)
		require.True(t, ok, "the preview should still be registered")

		if view.Done() {
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

func TestPreviewRegistryAllowsRetryingAFailedRender(t *testing.T) {
	t.Parallel()

	registry := newRegistry()

	require.Equal(
		t,
		AdmittedRender,
		registry.render(t.Context(), "p1", 4, func(context.Context) error {
			return errRender
		}),
		"the first attempt should start",
	)

	view := awaitTerminal(t, registry, "p1")
	require.Equal(t, "failed", string(view.Status))

	ran := make(chan struct{})

	require.Equal(
		t,
		AdmittedRender,
		registry.render(t.Context(), "p1", 4, func(context.Context) error {
			close(ran)

			return nil
		}),
		"a failed render must be retryable",
	)

	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the retry never ran")
	}

	retry := awaitTerminal(t, registry, "p1")
	assert.Equal(t, "completed", string(retry.Status))
}

func TestPreviewAdmissionRefusesWhenTheQueueIsFull(t *testing.T) {
	t.Parallel()

	service := New(2, nil, blob.Paths{}, nil)
	registry := service.entries
	slots := service.Slots()

	for i := range slots * queuedPerSlot {
		held := make(chan struct{})
		name := "p" + strconv.Itoa(i)

		require.Equal(
			t,
			AdmittedRender,
			registry.render(t.Context(), name, slots, func(ctx context.Context) error {
				close(held)
				<-ctx.Done()

				return ctx.Err()
			}),
			"render %d should be admitted",
			i,
		)

		<-held
	}

	assert.Equal(t, RefusedFull, registry.admit(newPendingJob("brand-new"), slots),
		"a new preview must be refused at the admission limit")

	spare, err := service.Acquire(t.Context(), time.Second)
	require.NoError(t, err)
	spare()

	assert.Equal(t, RefusedFull, registry.admit(newPendingJob("still-full"), slots),
		"a free gate slot must not admit a render past the limit")

	assert.Equal(t, AdmittedExisting, registry.admit(newPendingJob("p0"), slots),
		"a preview already rendering must be joined, not refused")

	require.True(t, registry.cancel("p0"))
	awaitTerminal(t, registry, "p0")

	assert.Equal(t, AdmittedRender, registry.admit(newPendingJob("now-free"), slots),
		"finishing a render must free an admission")

	for i := 1; i < slots*queuedPerSlot; i++ {
		registry.cancel("p" + strconv.Itoa(i))
	}
}

func TestPreviewRegistryRejectsARunningRender(t *testing.T) {
	t.Parallel()

	registry := newRegistry()
	release := make(chan struct{})

	require.Equal(
		t,
		AdmittedRender,
		registry.render(t.Context(), "p1", 4, func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		}),
	)

	require.Equal(
		t,
		AdmittedExisting,
		registry.render(t.Context(), "p1", 4, func(context.Context) error {
			close(release)

			return nil
		}),
		"a render in flight must keep its entry",
	)

	assert.True(t, registry.cancel("p1"))
}

func TestPreviewRegistryRecordsCancellationWhenProcessIsKilled(t *testing.T) {
	t.Parallel()

	registry := newRegistry()
	started := make(chan struct{})
	release := make(chan struct{})

	require.Equal(
		t,
		AdmittedRender,
		registry.render(t.Context(), "p1", 4, func(context.Context) error {
			close(started)
			<-release

			return errKilledProcess
		}),
	)

	<-started
	require.True(t, registry.cancel("p1"))
	close(release)

	view := awaitTerminal(t, registry, "p1")
	assert.Equal(t, "canceled", string(view.Status), "a deliberate cancel is not a failure")
	assert.Empty(t, view.Error, "the killed process error must not be shown to the user")
}

func TestPreviewRegistryKeepsRealFailures(t *testing.T) {
	t.Parallel()

	registry := newRegistry()

	require.Equal(
		t,
		AdmittedRender,
		registry.render(t.Context(), "p1", 4, func(context.Context) error {
			return errRender
		}),
	)

	view := awaitTerminal(t, registry, "p1")
	assert.Equal(t, "failed", string(view.Status))
	assert.Equal(t, errRender.Error(), view.Error)
}

func TestPreviewRegistryKeepsCompletionWhenCancelArrivesLate(t *testing.T) {
	t.Parallel()

	registry := newRegistry()
	published := make(chan struct{})
	release := make(chan struct{})

	require.Equal(
		t,
		AdmittedRender,
		registry.render(t.Context(), "p1", 4, func(context.Context) error {
			close(published)
			<-release

			return nil
		}),
	)

	<-published

	require.True(t, registry.cancel("p1"),
		"the job is still marked running until finish records it")
	close(release)

	view := awaitTerminal(t, registry, "p1")
	assert.Equal(t, "completed", string(view.Status),
		"a render that succeeded must not be reported as canceled")
	assert.Equal(t, progressDone, view.Progress)
	assert.Empty(t, view.Error)
}

func TestServicePublishedTracksTheFinalFile(t *testing.T) {
	t.Parallel()

	service, store := newServiceFixture(t, 1)

	assert.False(t, service.Published("never-rendered"))

	require.NoError(t, os.WriteFile(store.PreviewPath("p1"), []byte("preview"), 0o600))

	assert.True(t, service.Published("p1"))
}

func TestServiceRenderIntoPublishesUnderThePreviewID(t *testing.T) {
	t.Parallel()

	service, _ := newServiceFixture(t, 1)

	err := service.RenderInto(
		t.Context(),
		"p1",
		"/media/source.mkv",
		clip.Request{MediaID: "42", Duration: 5},
		false,
	)
	require.NoError(t, err)

	assert.True(t, service.Published("p1"), "the preview must be published under its id")
}

func TestServiceRenderIntoSkipsAPreviewThatExists(t *testing.T) {
	t.Parallel()

	service, store := newServiceFixture(t, 1)
	require.NoError(t, os.WriteFile(store.PreviewPath("p1"), []byte("earlier"), 0o600))

	err := service.RenderInto(
		t.Context(),
		"p1",
		"/media/source.mkv",
		clip.Request{MediaID: "42", Duration: 5},
		false,
	)
	require.NoError(t, err)

	assert.Empty(t, recordedTargets(t, store),
		"a render that queued behind an existing preview must not encode it again")

	contents, readErr := os.ReadFile(store.PreviewPath("p1"))
	require.NoError(t, readErr)
	assert.Equal(t, "earlier", string(contents), "the published preview must not be overwritten")
}
