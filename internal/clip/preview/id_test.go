// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
)

func writeKeySource(t *testing.T, start, duration float64) (string, clip.Request) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	return path, clip.Request{
		MediaID:    "42",
		StartTime:  start,
		Duration:   duration,
		AudioIndex: 1,
	}
}

func TestPreviewContentIDIsStable(t *testing.T) {
	t.Parallel()

	path, req := writeKeySource(t, 12.345, 20)

	first, err := RequestID(req, path)
	require.NoError(t, err)

	second, err := RequestID(req, path)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the same request must produce the same id")
}

func TestPreviewContentIDFitsTheStorageGuard(t *testing.T) {
	t.Parallel()

	path, req := writeKeySource(t, 1, 5)

	previewID, err := RequestID(req, path)
	require.NoError(t, err)

	assert.Len(t, previewID, 64, "a SHA-256 digest is 64 hex characters")
	assert.True(t, ValidID(previewID), "the derived id must pass the storage guard")
}

func TestPreviewContentIDSeparatesFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	absorbed := filepath.Join(dir, "a")
	require.NoError(t, os.WriteFile(absorbed, []byte("source"), 0o600))

	digit := filepath.Join(dir, "a1")
	require.NoError(t, os.WriteFile(digit, []byte("source"), 0o600))

	first, err := RequestID(
		clip.Request{MediaID: "42", StartTime: 12.345, Duration: 5},
		absorbed,
	)
	require.NoError(t, err)

	second, err := RequestID(
		clip.Request{MediaID: "42", StartTime: 2.345, Duration: 5},
		digit,
	)
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "a path must not be able to absorb the seek offset")
}

func TestPreviewContentIDChangesWithEveryInputField(t *testing.T) {
	t.Parallel()

	path, base := writeKeySource(t, 12.345, 20)

	baseID, err := RequestID(base, path)
	require.NoError(t, err)

	other := filepath.Join(t.TempDir(), "other.mkv")
	require.NoError(t, os.WriteFile(other, []byte("source"), 0o600))

	tests := []struct {
		name   string
		path   string
		mutate func(*clip.Request)
	}{
		{name: "start", mutate: func(r *clip.Request) { r.StartTime = 12.346 }},
		{name: "duration", mutate: func(r *clip.Request) { r.Duration = 21 }},
		{name: "audio index", mutate: func(r *clip.Request) { r.AudioIndex = 2 }},
		{name: "crop", mutate: func(r *clip.Request) { r.CropBlackBars = true }},
		{name: "preserve hdr", mutate: func(r *clip.Request) {
			on := true

			r.PreserveHDR = &on
		}},
		{name: "media id", mutate: func(r *clip.Request) { r.MediaID = "43" }},
		{name: "source path", path: other, mutate: func(*clip.Request) {}},
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

			got, idErr := RequestID(req, source)
			require.NoError(t, idErr)

			assert.NotEqual(t, baseID, got, "a changed %s must produce a different id", test.name)
		})
	}

	t.Run("source modification time", func(t *testing.T) {
		t.Parallel()

		restamp := filepath.Join(t.TempDir(), "restamp.mkv")
		require.NoError(t, os.WriteFile(restamp, []byte("source"), 0o600))
		require.NoError(
			t,
			os.Chtimes(restamp, time.Now().Add(time.Hour), time.Now().Add(time.Hour)),
		)

		before, idErr := RequestID(base, restamp)
		require.NoError(t, idErr)

		require.NoError(t, os.Chtimes(
			restamp,
			time.Now().Add(2*time.Hour),
			time.Now().Add(2*time.Hour),
		))

		after, idErr := RequestID(base, restamp)
		require.NoError(t, idErr)

		assert.NotEqual(t, before, after, "a re-transcoded source must produce a different id")
	})
}

func TestPreviewRequestIDHonoursTheWholeSelection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	longest, err := RequestID(
		clip.Request{MediaID: "42", StartTime: 1, Duration: 600},
		path,
	)
	require.NoError(t, err)

	short, err := RequestID(clip.Request{MediaID: "42", StartTime: 1, Duration: 30}, path)
	require.NoError(t, err)

	assert.NotEqual(t, longest, short, "each selection must render its own range")

	assert.InDelta(t, 600, ffmpeg.PreviewDuration(600), 0.0001,
		"the preview cap must not sit below the longest clip the form accepts")
}

func TestPreviewRequestIDIgnoresSubMillisecondStart(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	exact, err := RequestID(
		clip.Request{MediaID: "42", StartTime: 12.345, Duration: 5},
		path,
	)
	require.NoError(t, err)

	nudged, err := RequestID(
		clip.Request{MediaID: "42", StartTime: 12.3451, Duration: 5},
		path,
	)
	require.NoError(t, err)

	assert.Equal(t, exact, nudged, "a sub-millisecond start encodes the same frame")

	later, err := RequestID(
		clip.Request{MediaID: "42", StartTime: 12.346, Duration: 5},
		path,
	)
	require.NoError(t, err)

	assert.NotEqual(t, exact, later, "a different millisecond is a different frame")
}

func TestPreviewRequestIDRejectsAnUnreadableSource(t *testing.T) {
	t.Parallel()

	_, err := RequestID(
		clip.Request{MediaID: "42", Duration: 5},
		filepath.Join(t.TempDir(), "absent.mkv"),
	)

	require.ErrorIs(t, err, ErrSourceUnreadable)
}

func TestPreviewRequestIDFollowsTheSelection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))

	base := clip.Request{MediaID: "42", StartTime: 180, Duration: 10}

	first, err := RequestID(base, path)
	require.NoError(t, err)

	repeat, err := RequestID(base, path)
	require.NoError(t, err)
	assert.Equal(t, first, repeat, "the same selection must reuse its preview")

	tests := []struct {
		name   string
		mutate func(*clip.Request)
	}{
		{name: "start moved", mutate: func(r *clip.Request) { r.StartTime = 190 }},
		{name: "end moved", mutate: func(r *clip.Request) { r.Duration = 20 }},
		{
			name:   "window lengthened past the cap",
			mutate: func(r *clip.Request) { r.Duration = 600 },
		},
		{name: "audio track changed", mutate: func(r *clip.Request) { r.AudioIndex = 1 }},
		{name: "crop toggled", mutate: func(r *clip.Request) { r.CropBlackBars = true }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := base
			test.mutate(&req)

			got, idErr := RequestID(req, path)
			require.NoError(t, idErr)

			assert.NotEqual(t, first, got, "%s must ask for a different preview", test.name)
		})
	}
}

func TestValidPreviewID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want bool
	}{
		{name: "uuid", give: "1f0c3a52-8b6d-4e21-9a77-2c5d8e4f1b03", want: true},
		{
			name: "hex hash",
			give: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			want: true,
		},
		{name: "base64url hash", give: "aB3-_xYz09", want: true},
		{name: "single character", give: "a", want: true},
		{name: "empty", give: "", want: false},
		{name: "parent segment", give: "..", want: false},
		{name: "traversal", give: "../secret", want: false},
		{name: "encoded traversal", give: "..%2f..%2fetc%2fpasswd", want: false},
		{name: "encoded dot", give: "%2e%2e", want: false},
		{name: "double encoded", give: "%252e%252e%252fsecret", want: false},
		{name: "encoded backslash", give: "..%5c..%5csecret", want: false},
		{name: "null byte", give: "a%00b", want: false},
		{name: "slash", give: "a/b", want: false},
		{name: "backslash", give: `a\b`, want: false},
		{name: "space", give: "a b", want: false},
		{name: "dot", give: "a.b", want: false},
		{name: "tilde", give: "~a", want: false},
		{name: "percent", give: "a%b", want: false},
		{name: "colon", give: "a:b", want: false},
		{name: "too long", give: longPreviewID(65), want: false},
		{name: "at the limit", give: longPreviewID(64), want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, ValidID(test.give))
		})
	}
}

func longPreviewID(n int) string {
	return strings.Repeat("a", n)
}
