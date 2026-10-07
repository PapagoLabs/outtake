// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/blob"
)

type failingPutStore struct {
	*blob.Storage

	putErr    error
	deleteErr error
}

var (
	errUpload = errors.New("upload rejected")
	errRemove = errors.New("delete rejected")
)

func newStagingFixture(t *testing.T) (*blob.Storage, string) {
	t.Helper()

	store, err := blob.NewStorage(blob.NewPaths(t.TempDir()))
	require.NoError(t, err)

	return store, store.PreviewPath("content-hash")
}

func previewList(t *testing.T, store *blob.Storage) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(store.BasePath(), "previews"))
	if err != nil {
		return nil
	}

	names := make([]string, 0, len(entries))

	for _, item := range entries {
		if strings.HasSuffix(item.Name(), ".argv") {
			continue
		}

		names = append(names, item.Name())
	}

	return names
}

func renderStagedPreview(
	t *testing.T,
	store blob.Blob,
	encodeErr bool,
	final string,
) error {
	t.Helper()

	err := Render(
		t.Context(),
		store,
		stagingFFmpeg(t, encodeErr),
		"/media/source.mkv",
		final,
		clip.Request{MediaID: "42", Duration: 5},
		false,
	)
	if err != nil {
		return fmt.Errorf("render preview: %w", err)
	}

	return nil
}

func TestRenderPreviewStagesBeforePublishing(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)

	err := renderStagedPreview(t, store, false, final)
	require.NoError(t, err)

	targets := recordedTargets(t, store)
	require.Len(t, targets, 1, "the encode should run once")
	assert.NotEqual(t, final, targets[0], "ffmpeg must not write the final path directly")
	assert.Equal(t, filepath.Dir(final), filepath.Dir(targets[0]),
		"the staged file must share the final file's directory, so publishing is a rename")

	assert.FileExists(t, final, "the preview must be published under its final name")
	assert.Equal(t, []string{filepath.Base(final)}, previewList(t, store),
		"the staged file must not survive the publish")
}

func TestRenderPreviewStagedEncodeIsBrowserSafe(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)

	err := renderStagedPreview(t, store, false, final)
	require.NoError(t, err)

	targets := recordedTargets(t, store)
	require.Len(t, targets, 1)

	argv := recordedArgv(t, targets[0])
	joined := strings.Join(argv, " ")

	assert.Contains(t, argv, "libx264")
	assert.Contains(t, argv, "-pix_fmt")
	assert.Contains(t, argv, "yuv420p")
	assert.Contains(t, argv, "-preset")
	assert.Contains(t, argv, "ultrafast")
	assert.Contains(t, argv, "-movflags")
	assert.Contains(t, argv, "+faststart")
	assert.Contains(t, argv, "5.000", "the requested window must be passed through")
	assert.NotContains(t, joined, "write_colr", "an SDR source is not tone mapped")
	assert.Equal(t, targets[0], argv[len(argv)-1], "the output path must be the last argument")
}

func TestRenderPreviewRemovesStagedFileOnFailure(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)

	err := renderStagedPreview(t, store, true, final)
	requireRenderFailed(t, err, final)

	assert.NoFileExists(t, final, "a failed render must not publish a preview")
	assert.Empty(t, previewList(t, store), "a failed render must not leave a staged file")
}

func TestRenderPreviewKeepsAnEarlierPreviewOnFailure(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)
	require.NoError(t, os.WriteFile(final, []byte("earlier"), 0o600))

	err := renderStagedPreview(t, store, true, final)
	requireRenderFailed(t, err, final)

	contents, readErr := os.ReadFile(final)
	require.NoError(t, readErr)
	assert.Equal(t, "earlier", string(contents), "the published preview must not be overwritten")
	assert.Equal(t, []string{filepath.Base(final)}, previewList(t, store),
		"the staged file must be cleaned up")
}

func TestRenderPreviewStagingIsUniquePerRender(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)

	for range 2 {
		err := renderStagedPreview(t, store, false, final)
		require.NoError(t, err)
	}

	targets := recordedTargets(t, store)
	require.Len(t, targets, 2)
	assert.NotEqual(t, targets[0], targets[1], "each render must stage under its own name")
	assert.FileExists(t, final)
	assert.Equal(t, []string{filepath.Base(final)}, previewList(t, store))
}

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

func (store *failingPutStore) Put(context.Context, string) error {
	return store.putErr
}

func TestRenderPreviewRemovesAPublishedPreviewThatFailedToUpload(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)
	failing := &failingPutStore{Storage: store, putErr: errUpload}

	err := renderStagedPreview(t, failing, false, final)
	require.ErrorIs(t, err, errUpload)

	assert.NoFileExists(t, final, "a preview that never uploaded must not be a cache hit")
	assert.Empty(t, previewList(t, store), "neither the published nor staged file may remain")
}

func TestRenderPreviewReportsAFailedCleanup(t *testing.T) {
	t.Parallel()

	store, final := newStagingFixture(t)
	failing := &failingPutStore{Storage: store, putErr: errUpload, deleteErr: errRemove}

	err := renderStagedPreview(t, failing, false, final)
	require.ErrorIs(t, err, errUpload, "the upload failure must be preserved")
	require.ErrorIs(t, err, errRemove, "a cleanup that also failed must be reported")
}

func TestDiscardPublishedPreviewToleratesAMissingFile(t *testing.T) {
	t.Parallel()

	store, _ := newStagingFixture(t)
	absent := store.PreviewPath("never-published")

	assert.NotPanics(t, func() { DiscardPublished(store, absent) })
}

func requireRenderFailed(t *testing.T, err error, final string) {
	t.Helper()

	require.Error(t, err)
	require.ErrorContains(t, err, "preview "+filepath.Base(final),
		"the failure must name which preview it was writing")
	assert.True(t, exitError(err), "the failure must be the ffmpeg process's, not a substitute")
}
