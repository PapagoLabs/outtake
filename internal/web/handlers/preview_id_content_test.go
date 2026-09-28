// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/media"
)

// writeKeySource creates a source file and returns its path and a request
// describing a preview of it.
//
// Parameters:
//   - t: Test context.
//   - start: Seek offset in seconds.
//   - duration: Requested duration in seconds.
//
// Returns:
//   - path: Path of the created source.
//   - req: A request that previews it.
func writeKeySource(t *testing.T, start, duration float64) (string, api.ClipRequest) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	return path, api.ClipRequest{
		MediaID:    "42",
		StartTime:  start,
		Duration:   duration,
		AudioIndex: 1,
	}
}

// TestPreviewContentIDIsStable covers the property the whole cache rests on: the
// same request must always produce the same id, or nothing is ever reused.
func TestPreviewContentIDIsStable(t *testing.T) {
	t.Parallel()

	path, req := writeKeySource(t, 12.345, 20)

	first, err := previewRequestID(req, path)
	require.NoError(t, err)

	second, err := previewRequestID(req, path)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the same request must produce the same id")
}

// TestPreviewContentIDFitsTheStorageGuard checks the id the key produces is one
// the storage path guard from an earlier branch actually accepts. A digest that
// the guard rejects would 404 every preview it named.
func TestPreviewContentIDFitsTheStorageGuard(t *testing.T) {
	t.Parallel()

	path, req := writeKeySource(t, 1, 5)

	id, err := previewRequestID(req, path)
	require.NoError(t, err)

	assert.Len(t, id, 64, "a SHA-256 digest is 64 hex characters")
	assert.True(t, validPreviewID(id), "the derived id must pass the storage guard")
}

// TestPreviewContentIDSeparatesFields guards against an ambiguous encoding.
//
// The seam that can actually collide is path into seek: without a terminator a
// path ending in a digit absorbs the first digit of the seek offset, so
// "…/a" at 12.345 and "…/a1" at 2.345 would hash identically and one would be
// served for the other.
func TestPreviewContentIDSeparatesFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	absorbed := filepath.Join(dir, "a")
	require.NoError(t, os.WriteFile(absorbed, []byte("source"), 0o600))

	digit := filepath.Join(dir, "a1")
	require.NoError(t, os.WriteFile(digit, []byte("source"), 0o600))

	first, err := previewRequestID(
		api.ClipRequest{MediaID: "42", StartTime: 12.345, Duration: 5},
		absorbed,
	)
	require.NoError(t, err)

	second, err := previewRequestID(
		api.ClipRequest{MediaID: "42", StartTime: 2.345, Duration: 5},
		digit,
	)
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "a path must not be able to absorb the seek offset")
}

// TestPreviewContentIDChangesWithEveryInputField is the cache-correctness
// property: if any field that changes the ffmpeg command is left out of the
// key, a stale preview is served in its place.
func TestPreviewContentIDChangesWithEveryInputField(t *testing.T) {
	t.Parallel()

	path, base := writeKeySource(t, 12.345, 20)

	baseID, err := previewRequestID(base, path)
	require.NoError(t, err)

	other := filepath.Join(t.TempDir(), "other.mkv")
	require.NoError(t, os.WriteFile(other, []byte("source"), 0o600))

	// The source is deliberately left alone here. Restamping it after baseID
	// would change the modification time for every subtest, so each one would
	// pass on that alone and a field left out of the key would go unnoticed. The
	// modification time has its own case below, with its own file.

	tests := []struct {
		name   string
		path   string
		mutate func(*api.ClipRequest)
	}{
		{name: "start", mutate: func(r *api.ClipRequest) { r.StartTime = 12.346 }},
		{name: "duration", mutate: func(r *api.ClipRequest) { r.Duration = 21 }},
		{name: "audio index", mutate: func(r *api.ClipRequest) { r.AudioIndex = 2 }},
		{name: "crop", mutate: func(r *api.ClipRequest) { r.CropBlackBars = true }},
		{name: "web safe", mutate: func(r *api.ClipRequest) {
			on := true

			r.WebSafeColor = &on
		}},
		{name: "media id", mutate: func(r *api.ClipRequest) { r.MediaID = "43" }},
		{name: "source path", path: other, mutate: func(*api.ClipRequest) {}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := base
			test.mutate(&req)

			source := path
			if test.path != "" {
				source = test.path
			}

			got, idErr := previewRequestID(req, source)
			require.NoError(t, idErr)

			assert.NotEqual(t, baseID, got, "a changed %s must produce a different id", test.name)
		})
	}

	// The source's own identity is part of the key, so a re-transcoded file of
	// the same length at the same path is a different preview.
	t.Run("source modification time", func(t *testing.T) {
		t.Parallel()

		restamp := filepath.Join(t.TempDir(), "restamp.mkv")
		require.NoError(t, os.WriteFile(restamp, []byte("source"), 0o600))
		require.NoError(
			t,
			os.Chtimes(restamp, time.Now().Add(time.Hour), time.Now().Add(time.Hour)),
		)

		before, idErr := previewRequestID(base, restamp)
		require.NoError(t, idErr)

		require.NoError(t, os.Chtimes(
			restamp,
			time.Now().Add(2*time.Hour),
			time.Now().Add(2*time.Hour),
		))

		after, idErr := previewRequestID(base, restamp)
		require.NoError(t, idErr)

		assert.NotEqual(t, before, after, "a re-transcoded source must produce a different id")
	})
}

// TestPreviewRequestIDClampsTheWindow is the saving this branch exists for.
//
// Ffmpeg is given the clamped window, so a clip longer than the cap and one
// exactly at it run an identical command. Keying on the requested duration would
// miss on every clip worth previewing.
func TestPreviewRequestIDClampsTheWindow(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	atCap, err := previewRequestID(api.ClipRequest{MediaID: "42", StartTime: 1, Duration: 30}, path)
	require.NoError(t, err)

	beyond, err := previewRequestID(
		api.ClipRequest{MediaID: "42", StartTime: 1, Duration: 600},
		path,
	)
	require.NoError(t, err)

	assert.Equal(t, atCap, beyond, "a window past the cap encodes the same range")

	below, err := previewRequestID(api.ClipRequest{MediaID: "42", StartTime: 1, Duration: 5}, path)
	require.NoError(t, err)

	assert.NotEqual(t, atCap, below, "a window under the cap encodes something different")
	assert.InDelta(t, 30.0, media.PreviewDuration(600), 0.0001)
}

// TestPreviewRequestIDIgnoresSubMillisecondStart pins the quantisation to the
// resolution ffmpeg arguments are rendered at, so two values producing an
// identical command cannot fall into different keys.
func TestPreviewRequestIDIgnoresSubMillisecondStart(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	exact, err := previewRequestID(
		api.ClipRequest{MediaID: "42", StartTime: 12.345, Duration: 5},
		path,
	)
	require.NoError(t, err)

	nudged, err := previewRequestID(
		api.ClipRequest{MediaID: "42", StartTime: 12.3451, Duration: 5},
		path,
	)
	require.NoError(t, err)

	assert.Equal(t, exact, nudged, "a sub-millisecond start encodes the same frame")

	later, err := previewRequestID(
		api.ClipRequest{MediaID: "42", StartTime: 12.346, Duration: 5},
		path,
	)
	require.NoError(t, err)

	assert.NotEqual(t, exact, later, "a different millisecond is a different frame")
}

// TestPreviewRequestIDRejectsAnUnreadableSource covers a source that cannot be
// stat'd. It has no stable identity, so a key built without one would be wrong
// the moment the file is replaced.
func TestPreviewRequestIDRejectsAnUnreadableSource(t *testing.T) {
	t.Parallel()

	_, err := previewRequestID(
		api.ClipRequest{MediaID: "42", Duration: 5},
		filepath.Join(t.TempDir(), "absent.mkv"),
	)

	require.ErrorIs(t, err, errSourceUnreadable)
}
