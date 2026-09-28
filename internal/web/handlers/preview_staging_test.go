// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/media/mocks"
	"github.com/PapagoLabs/outtake/internal/storage"
)

// failingPutStore wraps a store so uploads, and optionally deletes, fail. It
// stands in for an object backend that rejects the write.
type failingPutStore struct {
	*storage.Storage

	putErr    error
	deleteErr error
}

// errUpload stands in for an object backend rejecting a write.
var (
	errUpload = errors.New("upload rejected")
	errRemove = errors.New("delete rejected")
)

// newStagingFixture builds a store and the final path a preview is published under.
//
// Parameters:
//   - t: Test context.
//
// Returns:
//   - store: A filesystem store over a temporary directory.
//   - final: The final path for a preview of these parameters.
func newStagingFixture(t *testing.T) (*storage.Storage, string) {
	t.Helper()

	store, err := storage.NewStorage(t.TempDir())
	require.NoError(t, err)

	return store, store.PreviewPath("content-hash")
}

// previewList returns the file names in a store's previews directory.
//
// Parameters:
//   - t: Test context.
//   - store: Store to inspect.
//
// Returns:
//   - names: File names present, or nil when the directory is absent.
func previewList(t *testing.T, store *storage.Storage) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(store.BasePath(), "previews"))
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

// encodingFFmpeg returns a mock that writes a marker file wherever it is asked
// to encode, unless encodeErr is set.
//
// Parameters:
//   - t: Test context.
//   - targets: Collects the paths it was asked to write, in order.
//   - encodeErr: When set, a partial file is written and this is returned
//     instead, standing in for a render that was cut short.
//
// Returns:
//   - ffmpeg: The mock.
func encodingFFmpeg(
	t *testing.T,
	targets *[]string,
	encodeErr error,
) *mocks.MockFFmpeg {
	t.Helper()

	ffmpeg := mocks.NewMockFFmpeg(t)
	ffmpeg.EXPECT().
		DetectCrop(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(media.CropRect{}, nil).
		Maybe()
	ffmpeg.EXPECT().
		ExtractPreview(mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context,
			_ string,
			output string,
			_ float64,
			_ float64,
			_ int,
			_ media.CropRect,
			_ media.QualityPreset,
		) error {
			*targets = append(*targets, output)

			// A render that is cut short still leaves a partial file behind,
			// which is the case the staged cleanup exists for.
			if encodeErr != nil {
				_ = os.WriteFile(output, []byte("partial"), 0o600)

				return encodeErr
			}

			return os.WriteFile(output, []byte("encoded"), 0o600)
		}).
		Maybe()

	return ffmpeg
}

// TestRenderPreviewStagesBeforePublishing is the regression guard.
//
// The final path is derived from the request, so writing straight to it would
// let two renders of the same parameters write one file at once, and would
// leave a partial file where a finished preview is expected.
func TestRenderPreviewStagesBeforePublishing(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)

	var targets []string

	err := renderPreview(
		t.Context(),
		store,
		encodingFFmpeg(t, &targets, nil),
		"/media/source.mkv",
		final,
		api.ClipRequest{MediaID: "42", Duration: 5},
	)
	require.NoError(t, err)

	require.Len(t, targets, 1, "the encode should run once")
	assert.NotEqual(t, final, targets[0], "ffmpeg must not write the final path directly")
	assert.Equal(t, filepath.Dir(final), filepath.Dir(targets[0]),
		"the staged file must share the final file's directory, so publishing is a rename")

	assert.FileExists(t, final, "the preview must be published under its final name")
	assert.Equal(t, []string{filepath.Base(final)}, previewList(t, store),
		"the staged file must not survive the publish")
}

// TestRenderPreviewRemovesStagedFileOnFailure covers a failed encode leaving a
// partial file where a preview would be served from.
func TestRenderPreviewRemovesStagedFileOnFailure(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)

	var targets []string

	ffmpeg := encodingFFmpeg(t, &targets, errRender)

	err := renderPreview(
		t.Context(),
		store,
		ffmpeg,
		"/media/source.mkv",
		final,
		api.ClipRequest{MediaID: "42", Duration: 5},
	)
	require.ErrorIs(t, err, errRender)

	assert.NoFileExists(t, final, "a failed render must not publish a preview")
	assert.Empty(t, previewList(t, store), "a failed render must not leave a staged file")
}

// TestRenderPreviewKeepsAnEarlierPreviewOnFailure guards the case where a
// preview is already published and a later render fails. The published file is
// still valid, so it must survive.
func TestRenderPreviewKeepsAnEarlierPreviewOnFailure(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)
	require.NoError(t, os.WriteFile(final, []byte("earlier"), 0o600))

	var targets []string

	ffmpeg := encodingFFmpeg(t, &targets, errRender)

	err := renderPreview(
		t.Context(),
		store,
		ffmpeg,
		"/media/source.mkv",
		final,
		api.ClipRequest{MediaID: "42", Duration: 5},
	)
	require.ErrorIs(t, err, errRender)

	contents, readErr := os.ReadFile(final)
	require.NoError(t, readErr)
	assert.Equal(t, "earlier", string(contents), "the published preview must not be overwritten")
	assert.Equal(t, []string{filepath.Base(final)}, previewList(t, store),
		"the staged file must be cleaned up")
}

// TestRenderPreviewStagingIsUniquePerRender confirms two renders of the same
// parameters never share a staged path, which is the collision staging exists to
// prevent.
func TestRenderPreviewStagingIsUniquePerRender(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)

	var targets []string

	ffmpeg := encodingFFmpeg(t, &targets, nil)

	for range 2 {
		err := renderPreview(
			t.Context(),
			store,
			ffmpeg,
			"/media/source.mkv",
			final,
			api.ClipRequest{MediaID: "42", Duration: 5},
		)
		require.NoError(t, err)
	}

	require.Len(t, targets, 2)
	assert.NotEqual(t, targets[0], targets[1], "each render must stage under its own name")
	assert.FileExists(t, final)
	assert.Equal(t, []string{filepath.Base(final)}, previewList(t, store))
}

// DeleteFile always fails when a delete error is set.
func (store *failingPutStore) DeleteFile(path string) error {
	if store.deleteErr != nil {
		return store.deleteErr
	}

	err := store.Storage.DeleteFile(path)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	return nil
}

// Put always fails.
func (store *failingPutStore) Put(context.Context, string) error {
	return store.putErr
}

// TestRenderPreviewRemovesAPublishedPreviewThatFailedToUpload covers the rename
// succeeding and the upload failing.
//
// The rename is what makes the file a cache hit, so leaving it behind would let
// every later request reuse a preview that never reached the bucket. It would
// then vanish on restart, or from another instance, with nothing to regenerate
// it on the strength of.
func TestRenderPreviewRemovesAPublishedPreviewThatFailedToUpload(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)
	failing := &failingPutStore{Storage: store, putErr: errUpload}

	var targets []string

	err := renderPreview(
		t.Context(),
		failing,
		encodingFFmpeg(t, &targets, nil),
		"/media/source.mkv",
		final,
		api.ClipRequest{MediaID: "42", Duration: 5},
	)
	require.ErrorIs(t, err, errUpload)

	assert.NoFileExists(t, final, "a preview that never uploaded must not be a cache hit")
	assert.Empty(t, previewList(t, store), "neither the published nor staged file may remain")
}

// TestRenderPreviewReportsAFailedCleanup checks a cleanup that also fails is
// surfaced, rather than leaving a published preview that never uploaded looking
// like a valid cache hit.
func TestRenderPreviewReportsAFailedCleanup(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)
	failing := &failingPutStore{Storage: store, putErr: errUpload, deleteErr: errRemove}

	var targets []string

	err := renderPreview(
		t.Context(),
		failing,
		encodingFFmpeg(t, &targets, nil),
		"/media/source.mkv",
		final,
		api.ClipRequest{MediaID: "42", Duration: 5},
	)
	require.ErrorIs(t, err, errUpload, "the upload failure must be preserved")
	require.ErrorIs(t, err, errRemove, "a cleanup that also failed must be reported")
}

// TestDiscardPublishedPreviewToleratesAMissingFile covers the cleanup running
// when the published file is already gone, which must not turn one failure into
// two.
func TestDiscardPublishedPreviewToleratesAMissingFile(t *testing.T) {
	t.Parallel()

	store, _ := newStagingFixture(t)
	absent := store.PreviewPath("never-published")

	assert.NotPanics(t, func() { discardPublishedPreview(store, absent) })
}

// TestDiscardStagedPreviewToleratesAMissingFile covers the cleanup path running
// when ffmpeg never created anything, which must not turn one failure into two.
func TestDiscardStagedPreviewToleratesAMissingFile(t *testing.T) {
	t.Parallel()

	absent := filepath.Join(t.TempDir(), "never-written.mp4")

	assert.NotPanics(t, func() { discardStagedPreview(absent) })
}
