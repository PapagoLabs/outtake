// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// copyProfileID is the stored profile the copy tests render with.
const copyProfileID = "copy-profile"

// copyJob builds a video clip on a real source, rendered with a profile that
// copies.
//
// The source's keyframes are three seconds apart, and the clip runs from 7.3
// to 9.3 seconds.
//
// Parameters:
//   - t: The test the clip belongs to.
//   - encode: The source's codec arguments.
//   - name: The source's file name, which picks its container.
//
// Returns:
//   - job: The clip.
//   - runner: An FFmpeg runner on the real binaries.
//   - db: The database holding the copying profile.
func copyJob(
	t *testing.T,
	encode []string,
	name string,
) (*clip.Job, *ffmpeg.ExecFFmpeg, *database.DB) {
	t.Helper()

	ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobePath, ffprobeErr := exec.LookPath("ffprobe")

	if ffmpegErr != nil || ffprobeErr != nil {
		t.Skip("ffmpeg and ffprobe are not available")
	}

	dir := t.TempDir()
	input := filepath.Join(dir, name)

	args := append([]string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "12",
	}, encode...)

	args = append(args, "-c:a", "aac", "-ac", "2", input)

	out, err := exec.CommandContext(t.Context(), ffmpegPath, args...).CombinedOutput()
	require.NoError(t, err, string(out))

	db := testDatabase(t)
	require.NoError(t, db.SaveClipProfile(t.Context(), database.ClipProfile{
		ID: copyProfileID, Name: "Copy", CRF: 20, Preset: "veryfast", AudioKbps: 128,
		MaxWidth: clip.OutputWidth1080p, CopyVideo: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))

	job := testClipJob("copied")

	job.Quality = copyProfileID
	job.InputPath = input
	job.OutputPath = filepath.Join(dir, "clip.mp4")
	job.StartTime = 7300 * time.Millisecond
	job.Duration = 2 * time.Second

	return job, ffmpeg.NewExecFFmpeg(ffmpegPath, ffprobePath), db
}

// copyStore is a blob store that accepts the clip and has no SDR version.
//
// Parameters:
//   - t: The test the store belongs to.
//   - job: The clip.
//
// Returns:
//   - store: The store mock.
func copyStore(t *testing.T, job *clip.Job) *storagemocks.MockBlob {
	t.Helper()

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Put(mock.Anything, job.OutputPath).Return(nil).Once()
	store.EXPECT().DeleteFile(mock.Anything).Return(nil).Maybe()

	return store
}

// TestProcessJobCopiesAClipThatFits covers a clip whose source already fits
// its profile.
//
// The clip is copied from the keyframe at six seconds, and its window moves
// there, reported once the file is published.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobCopiesAClipThatFits(t *testing.T) {
	job, runner, db := copyJob(
		t,
		[]string{"-c:v", "libx264", "-preset", "medium", "-g", "72", "-pix_fmt", "yuv420p"},
		"source.mp4",
	)

	stages := &recordedStages{}

	require.NoError(
		t,
		processJob(t.Context(), job, runner, db, copyStore(t, job), stages, clip.MaxDuration),
	)

	assert.Equal(t, 6*time.Second, job.StartTime, "the clip starts on the keyframe")
	assert.Equal(t, 3300*time.Millisecond, job.Duration, "and still ends at its end mark")

	start, duration := stages.window()
	assert.Equal(t, job.StartTime, start, "the moved window is reported")
	assert.Equal(t, job.Duration, duration)

	copied, err := probe.Probe(t.Context(), "ffprobe", job.OutputPath)
	require.NoError(t, err)
	assert.Equal(t, "h264", copied.VideoCodec)
	assert.InDelta(t, 3.3, copied.Duration.Seconds(), 0.5)
}

// TestProcessJobEncodesAClipWhoseCopyIsRefused covers a source a copy cannot
// start cleanly on.
//
// An open GOP keyframe's leading frames refer to pictures before the cut, so
// the copy is refused, and the clip is encoded with its marks as they are.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobEncodesAClipWhoseCopyIsRefused(t *testing.T) {
	job, runner, db := copyJob(t, []string{
		"-c:v",
		"libx264",
		"-preset",
		"medium",
		"-g",
		"72",
		"-x264-params",
		"open-gop=1",
		"-pix_fmt",
		"yuv420p",
	}, "source.mkv")

	stages := &recordedStages{}

	require.NoError(
		t,
		processJob(t.Context(), job, runner, db, copyStore(t, job), stages, clip.MaxDuration),
	)

	assert.Equal(t, 7300*time.Millisecond, job.StartTime, "an encoded clip keeps its start mark")

	start, _ := stages.window()
	assert.Equal(t, 7300*time.Millisecond, start)
	assert.FileExists(t, job.OutputPath)
}

// TestProcessJobEncodesAClipACopyWouldStretch covers a copy past the limit.
//
// Starting on the keyframe would make the clip longer than the installation
// accepts, so it is encoded instead.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobEncodesAClipACopyWouldStretch(t *testing.T) {
	job, runner, db := copyJob(
		t,
		[]string{"-c:v", "libx264", "-preset", "medium", "-g", "72", "-pix_fmt", "yuv420p"},
		"source.mp4",
	)

	stages := &recordedStages{}

	require.NoError(
		t,
		processJob(t.Context(), job, runner, db, copyStore(t, job), stages, 3*time.Second),
	)

	assert.Equal(t, 7300*time.Millisecond, job.StartTime, "the clip keeps its start mark")
}
