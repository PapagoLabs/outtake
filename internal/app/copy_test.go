// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

const (
	// copyProfileID is the stored profile the copy tests render with.
	copyProfileID = "copy-profile"
	// keyframePackets is a packet list with keyframes at 3 and 6 seconds.
	keyframePackets = "3.000000,K__\n4.000000,___\n6.000000,K__\n"
)

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

// fakeCopyRunner installs fakes that answer each call a copied render makes.
//
// The source probes as 640 pixels wide in format, with keyframes at 3 and 6
// seconds, and every seek lands on the 6 second one. The copy's length probe
// reports length.
//
// Parameters:
//   - t: The test the fakes belong to.
//   - format: The source's pixel format.
//   - length: The copy's probed length, in seconds.
//   - packets: The packet list the keyframe search reads.
//
// Returns:
//   - runner: A runner on the fakes.
func fakeCopyRunner(t *testing.T, format, length, packets string) *ffmpeg.ExecFFmpeg {
	t.Helper()

	ffmpegPath := ffmpegtest.Install(t, ffmpegtest.Stub{
		Output: "rendered",
		Cases: []ffmpegtest.Case{
			{
				Arg:    "framecrc",
				Stdout: "#tb 0: 1/1000\n0,       5917,       6000,       41,    100, 0x0\n",
			},
			{Arg: "null"},
		},
	})

	ffprobePath := ffmpegtest.Install(t, ffmpegtest.Stub{
		Stdout: `{"format": {"duration": "` + length + `"}, "streams": [{"codec_type": "video", ` +
			`"codec_name": "h264", "width": 640, "height": 360, "pix_fmt": "` + format + `"}]}`,
		Cases: []ffmpegtest.Case{{Arg: "csv=p=0", Stdout: packets}},
	})

	return ffmpeg.NewExecFFmpeg(ffmpegPath, ffprobePath)
}

// fakeCopyJob builds a video clip from 7.3 to 9.3 seconds on a stub source,
// rendered with a profile that copies.
//
// Parameters:
//   - t: The test the clip belongs to.
//
// Returns:
//   - job: The clip.
//   - db: The database holding the copying profile.
func fakeCopyJob(t *testing.T) (*clip.Job, *database.DB) {
	t.Helper()

	dir := t.TempDir()

	db := testDatabase(t)
	require.NoError(t, db.SaveClipProfile(t.Context(), database.ClipProfile{
		ID: copyProfileID, Name: "Copy", CRF: 20, Preset: "veryfast", AudioKbps: 128,
		MaxWidth: clip.OutputWidth1080p, CopyVideo: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))

	job := testClipJob("copied-on-fakes")

	job.Quality = copyProfileID
	job.InputPath = stubInputFile(t, dir, "source.mp4")
	job.OutputPath = filepath.Join(dir, "clip.mp4")
	job.StartTime = 7300 * time.Millisecond
	job.Duration = 2 * time.Second

	return job, db
}

// TestProcessJobCopiesOnFakes covers a copied render without a real FFmpeg.
//
// The clip starts on the 6 second keyframe, and the moved window is reported
// once the file is published.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobCopiesOnFakes(t *testing.T) {
	job, db := fakeCopyJob(t)
	stages := &recordedStages{}

	runner := fakeCopyRunner(t, "yuv420p", "3.3", keyframePackets)

	require.NoError(
		t,
		processJob(t.Context(), job, runner, db, copyStore(t, job), stages, clip.MaxDuration),
	)

	start, duration := stages.window()
	assert.Equal(t, 6*time.Second, start)
	assert.Equal(t, 3300*time.Millisecond, duration)
}

// TestProcessJobEncodesARefusedCopyOnFakes covers a copy whose length is off.
//
// The copy is refused, the clip is encoded, and its marks stay as they are.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobEncodesARefusedCopyOnFakes(t *testing.T) {
	job, db := fakeCopyJob(t)
	stages := &recordedStages{}

	runner := fakeCopyRunner(t, "yuv420p", "6.3", keyframePackets)

	require.NoError(
		t,
		processJob(t.Context(), job, runner, db, copyStore(t, job), stages, clip.MaxDuration),
	)

	start, _ := stages.window()
	assert.Equal(t, 7300*time.Millisecond, start)
	assert.FileExists(t, job.OutputPath)
}

// TestPlanCopyRulesACopyOut covers each reason a clip is encoded rather than
// copied.
func TestPlanCopyRulesACopyOut(t *testing.T) {
	t.Parallel()

	copying := clip.QualityPreset{
		CRF: 20, Preset: "veryfast", AudioKbps: 128, MaxWidth: clip.OutputWidth1080p,
		PreserveHDR: false, CopyVideo: true,
	}

	tests := []struct {
		name        string
		format      string
		packets     string
		limit       time.Duration
		brokenProbe bool
		fits        bool
	}{
		{
			name:    "a source that fits",
			format:  "yuv420p",
			packets: keyframePackets,
			limit:   clip.MaxDuration,
			fits:    true,
		},
		{
			name:        "a source that cannot be probed",
			format:      "yuv420p",
			packets:     keyframePackets,
			limit:       clip.MaxDuration,
			brokenProbe: true,
		},
		{
			name:    "a source the encode would change",
			format:  "yuv420p10le",
			packets: keyframePackets,
			limit:   clip.MaxDuration,
		},
		{name: "no keyframe close enough", format: "yuv420p", packets: "", limit: clip.MaxDuration},
		{
			name:    "a copy past the limit",
			format:  "yuv420p",
			packets: keyframePackets,
			limit:   3 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			job := testClipJob("planned")

			job.InputPath = "/media/source.mp4"
			job.StartTime = 7300 * time.Millisecond
			job.Duration = 2 * time.Second

			runner := fakeCopyRunner(t, test.format, "3.3", test.packets)
			if test.brokenProbe {
				runner = ffmpeg.NewExecFFmpeg(
					"ffmpeg",
					ffmpegtest.Install(t, ffmpegtest.Stub{ExitCode: 1}),
				)
			}

			keyframe, _, fits := planCopy(
				t.Context(),
				job,
				runner,
				copying,
				crop.CropRect{},
				test.limit,
			)
			assert.Equal(t, test.fits, fits)

			if fits {
				assert.Equal(t, 6*time.Second, keyframe)
			}
		})
	}
}

// TestCopyVideoClipReportsAStoppedJob covers a job stopped before its copy,
// which is reported rather than encoded.
func TestCopyVideoClipReportsAStoppedJob(t *testing.T) {
	t.Parallel()

	job, _ := fakeCopyJob(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	copied, err := copyVideoClip(
		ctx,
		job,
		fakeCopyRunner(t, "yuv420p", "3.3", keyframePackets),
		clip.QualityPreset{
			CopyVideo: true,
			MaxWidth:  clip.OutputWidth1080p,
		},
		crop.CropRect{},
		clip.MaxDuration,
	)
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, copied)
}
