// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
)

// pqProbeJSON is an ffprobe answer for a PQ source, so a clip that keeps HDR
// renders an SDR version as well.
const pqProbeJSON = `{"format":{"duration":"120.0","bit_rate":"8000","format_name":"matroska"},` +
	`"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","width":3840,` +
	`"height":2160,"color_transfer":"smpte2084"},{"index":1,"codec_type":"audio",` +
	`"codec_name":"aac","channels":2}]}` + "\n"

// errStoreFailed is the failure the storage double reports.
var errStoreFailed = errors.New("upload rejected")

// testClipJob builds a clip job for the job and worker tests.
//
// Parameters:
//   - id: Identifier of the job.
//
// Returns:
//   - job: A pending clip job with no paths set.
func testClipJob(id string) *clip.Job {
	return &clip.Job{
		ID:         id,
		Type:       clip.TypeClip,
		Name:       id,
		MediaID:    "media-1",
		MediaTitle: "Test Movie",
		MediaType:  "movie",

		StartTime:     10 * time.Second,
		Duration:      15 * time.Second,
		Quality:       "medium",
		Width:         0,
		FPS:           0,
		AudioIndex:    1,
		CropBlackBars: false,
		PreserveHDR:   false,

		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second), InputPath: "",
		OutputPath: "",

		Status:   clip.StatusPending,
		Progress: 0,
		Error:    "",
	}
}

func TestExtractJobRejectsAnUnknownType(t *testing.T) {
	t.Parallel()

	job := testClipJob("unknown-type")

	job.Type = clip.Type("video")

	err := extractWithCrop(t, job, nil)
	require.ErrorIs(t, err, errUnknownClipType)
	assert.ErrorContains(t, err, "video", "the rejection names the type it refused")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobRendersAClip(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")

	job := testClipJob("clip-render")

	job.InputPath = stubInputFile(t, dir, "clip.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, logPath, ffmpegtest.Stub{}),
		missingBinary(dir),
	)

	require.NoError(t, extractWithCrop(t, job, execFFmpeg))

	invocations := stubInvocations(t, logPath)
	require.Len(t, invocations, 1, "a clip without crop trimming runs one pass")
	assertStagedFor(t, job.OutputPath, stubOutputArg(invocations[0]))
	assert.Contains(t, invocations[0], "-crf", "the clip arm renders an H.264 stream")
	assert.Contains(t, invocations[0], "libx264")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobRendersAClipWithADetectedCrop(t *testing.T) {
	// A fake whose stderr carries a cropdetect result.
	cropdetectSucceeds := ffmpegtest.Stub{
		Stderr: "Stream #0:0: Video: hevc, yuv420p, 1920x1080\n" +
			"[Parsed_cropdetect_0 @ 0x1] x1:0 x2:1919 y1:140 y2:939 w:1920 h:800 crop=1920:800:0:140\n",
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")

	job := testClipJob("clip-crop")

	job.InputPath = stubInputFile(t, dir, "crop.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")
	job.CropBlackBars = true

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, logPath, cropdetectSucceeds),
		missingBinary(dir),
	)

	require.NoError(t, extractWithCrop(t, job, execFFmpeg))

	invocations := stubInvocations(t, logPath)
	require.Len(t, invocations, 2, "crop detection runs before the encode")

	assert.True(t, stubArgvContains(invocations[0], "cropdetect=limit=24/255"),
		"the first pass is the cropdetect sampling: %v", invocations[0])

	encodeArgv := invocations[1]
	assert.True(t, stubArgvContains(encodeArgv, "crop=1920:800:0:140"),
		"the detected crop reaches the encode")

	rect, found := parseCropFilter(stubFilterArg(encodeArgv))
	require.True(t, found)
	assert.Equal(t, crop.CropRect{Width: 1920, Height: 800, X: 0, Y: 140}, rect)

	assertStagedFor(t, job.OutputPath, stubOutputArg(encodeArgv))
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobReportsAClipEncodeFailure(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("clip-failure")

	job.InputPath = stubInputFile(t, dir, "failure.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{ExitCode: 1}),
		missingBinary(dir),
	)

	err := extractWithCrop(t, job, execFFmpeg)
	require.ErrorContains(t, err, "extract clip")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobRendersAGIF(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")

	job := testClipJob("gif-render")

	job.Type = clip.TypeGIF
	job.InputPath = stubInputFile(t, dir, "gif.mkv")
	job.OutputPath = filepath.Join(dir, "clip.gif")
	job.Width = 480
	job.FPS = 12

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, logPath, ffmpegtest.Stub{}),
		missingBinary(dir),
	)

	require.NoError(t, extractWithCrop(t, job, execFFmpeg))

	invocations := stubInvocations(t, logPath)
	require.Len(t, invocations, 2, "a GIF runs a palette pass and an encode pass")

	assert.Equal(t, stubOutputArg(invocations[1])+".palette.png", stubOutputArg(invocations[0]),
		"the palette sits beside the staging file it is used for")
	assert.True(t, stubArgvContains(invocations[0], "palettegen=stats_mode=diff"))

	assertStagedFor(t, job.OutputPath, stubOutputArg(invocations[1]))
	assert.True(t, stubArgvContains(invocations[1], "paletteuse=dither=bayer"))
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobReportsAGIFPaletteFailure(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("gif-palette-failure")

	job.Type = clip.TypeGIF
	job.InputPath = stubInputFile(t, dir, "gif-palette.mkv")
	job.OutputPath = filepath.Join(dir, "clip.gif")

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{ExitCode: 1}),
		missingBinary(dir),
	)

	err := extractWithCrop(t, job, execFFmpeg)
	require.ErrorContains(t, err, "extract gif")
	assert.ErrorContains(t, err, "palettegen", "the failing pass names itself")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobRendersAScreenshot(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")

	job := testClipJob("screenshot-render")

	job.Type = clip.TypeScreenshot
	job.InputPath = stubInputFile(t, dir, "still.mkv")
	job.OutputPath = filepath.Join(dir, "still.png")

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, logPath, ffmpegtest.Stub{}),
		missingBinary(dir),
	)

	require.NoError(t, extractWithCrop(t, job, execFFmpeg))

	invocations := stubInvocations(t, logPath)
	require.Len(t, invocations, 1, "a screenshot needs no second pass")
	assertStagedFor(t, job.OutputPath, stubOutputArg(invocations[0]))
	assert.Contains(t, invocations[0], "-frames:v", "the screenshot arm grabs a single frame")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobReportsAScreenshotFailure(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("screenshot-failure")

	job.Type = clip.TypeScreenshot
	job.InputPath = stubInputFile(t, dir, "still-failure.mkv")
	job.OutputPath = filepath.Join(dir, "still.png")

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{ExitCode: 1}),
		missingBinary(dir),
	)

	err := extractWithCrop(t, job, execFFmpeg)
	require.ErrorContains(t, err, "extract screenshot")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobUploadsTheRenderedOutput(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("uploaded")

	job.InputPath = stubInputFile(t, dir, "upload.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Put(t.Context(), job.OutputPath).Return(nil).Once()
	store.EXPECT().DeleteFile(job.SDRPath()).Return(nil).Once()

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{}),
		missingBinary(dir),
	)

	stages := &recordedStages{}

	require.NoError(t, processJob(t.Context(), job, execFFmpeg, nil, store, stages))
	assert.Empty(t, stages.reported(), "a clip that converts to SDR has no second encode")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobReportsAnUploadFailure(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("upload-failure")

	job.InputPath = stubInputFile(t, dir, "upload-failure.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Put(t.Context(), job.OutputPath).Return(errStoreFailed).Once()

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{}),
		missingBinary(dir),
	)

	err := processJob(t.Context(), job, execFFmpeg, nil, store, &recordedStages{})
	require.ErrorIs(t, err, errStoreFailed)
	assert.ErrorContains(t, err, "store output")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobRefusesAJobWithoutAnOutput(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("no-output")

	job.InputPath = stubInputFile(t, dir, "no-output.mkv")

	store := storagemocks.NewMockBlob(t)

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{}),
		missingBinary(dir),
	)

	err := processJob(t.Context(), job, execFFmpeg, nil, store, &recordedStages{})
	require.ErrorIs(t, err, ffmpeg.ErrNoOutputPath,
		"a job with nowhere to write is refused before anything renders or uploads")
	assert.Empty(t, stubInvocations(t, filepath.Join(dir, "argv.log")), "ffmpeg never runs")
}

func TestProcessJobReportsAnExtractFailureBeforeUploading(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	job := testClipJob("extract-failure")

	job.Type = clip.Type("video")
	job.InputPath = stubInputFile(t, dir, "never-rendered.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	store := storagemocks.NewMockBlob(t)

	err := processJob(t.Context(), job, nil, nil, store, &recordedStages{})
	require.ErrorIs(t, err, errUnknownClipType)
	assert.ErrorContains(t, err, "extract")
}

func TestDetectJobCropSkipsWhenTrimmingIsOff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	job := testClipJob("no-trim")

	job.InputPath = stubInputFile(t, dir, "no-trim.mkv")
	job.CropBlackBars = false

	logPath := filepath.Join(dir, "argv.log")
	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, logPath, ffmpegtest.Stub{}),
		missingBinary(dir),
	)

	rect := detectJobCrop(t.Context(), execFFmpeg, job)

	assert.Equal(t, crop.CropRect{}, rect)
	assert.Empty(t, stubInvocations(t, logPath), "no detection pass was run at all")
}

//nolint:paralleltest // The detection pass reads the process-global logger New rewrites.
func TestDetectJobCropReturnsTheDetectedRectangle(t *testing.T) {
	// A fake whose stderr carries a cropdetect result.
	cropdetectSucceeds := ffmpegtest.Stub{
		Stderr: "Stream #0:0: Video: hevc, yuv420p, 1920x1080\n" +
			"[Parsed_cropdetect_0 @ 0x1] x1:0 x2:1919 y1:140 y2:939 w:1920 h:800 crop=1920:800:0:140\n",
	}

	dir := t.TempDir()

	job := testClipJob("trim")

	job.InputPath = stubInputFile(t, dir, "trim.mkv")
	job.CropBlackBars = true

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), cropdetectSucceeds),
		missingBinary(dir),
	)

	rect := detectJobCrop(t.Context(), execFFmpeg, job)

	assert.Equal(t, crop.CropRect{Width: 1920, Height: 800, X: 0, Y: 140}, rect)
}

//nolint:paralleltest // The detection pass reads the process-global logger New rewrites.
func TestDetectJobCropReportsADetectionFailureAsNoCrop(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("trim-failure")

	job.InputPath = stubInputFile(t, dir, "trim-failure.mkv")
	job.CropBlackBars = true

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{ExitCode: 1}),
		missingBinary(dir),
	)

	rect := detectJobCrop(t.Context(), execFFmpeg, job)

	assert.Equal(
		t,
		crop.CropRect{},
		rect,
		"a failed pass trims nothing rather than failing the job",
	)
}

// stubFilterArg returns the value that follows the video filter flag in an
// argv.
//
// Parameters:
//   - argv: One recorded invocation.
//
// Returns:
//   - filter: The -vf value, empty when the argv carries no filter.
func stubFilterArg(argv []string) string {
	const filterFlag = "-vf"

	for index, arg := range argv {
		if arg == filterFlag && index+1 < len(argv) {
			return argv[index+1]
		}
	}

	return ""
}

// parseCropFilter reads a crop=W:H:X:Y filter argument.
//
// Parameters:
//   - filter: The filter chain to read.
//
// Returns:
//   - rect: The rectangle the chain describes.
//   - found: False when the chain carries no crop filter.
func parseCropFilter(filter string) (crop.CropRect, bool) {
	_, cropFilter, found := strings.Cut(filter, "crop=")
	if !found {
		return crop.CropRect{}, false
	}

	cropFilter, _, _ = strings.Cut(cropFilter, ",")

	fields := strings.Split(cropFilter, ":")
	if len(fields) != 4 {
		return crop.CropRect{}, false
	}

	var numbers [4]int

	for index, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil {
			return crop.CropRect{}, false
		}

		numbers[index] = value
	}

	return crop.CropRect{
		Width:  numbers[0],
		Height: numbers[1],
		X:      numbers[2],
		Y:      numbers[3],
	}, true
}

// hdrClipJob is a video clip that keeps HDR, cut from a PQ source the probe
// stub reports, with its own output in dir.
//
// Parameters:
//   - t: The test the clip belongs to.
//   - dir: Directory the source and the outputs live in.
//
// Returns:
//   - job: The clip.
//   - runner: An FFmpeg runner whose ffmpeg logs to argv.log in dir.
func hdrClipJob(t *testing.T, dir string) (*clip.Job, *ffmpeg.ExecFFmpeg) {
	t.Helper()

	job := testClipJob("hdr-clip")

	job.PreserveHDR = true
	job.InputPath = stubInputFile(t, dir, "hdr.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	return job, ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{}),
		ffmpegtest.Install(t, ffmpegtest.Stub{Stdout: pqProbeJSON}),
	)
}

// TestProcessJobRendersAnSDRVersionOfAClipThatKeepsHDR covers the second
// encode: after the clip's own HEVC file, the render reports the SDR stage
// and encodes a tone mapped H.264 version beside the clip, and both files are
// uploaded.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobRendersAnSDRVersionOfAClipThatKeepsHDR(t *testing.T) {
	dir := t.TempDir()
	job, runner := hdrClipJob(t, dir)

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Put(mock.Anything, job.OutputPath).Return(nil).Once()
	store.EXPECT().Put(mock.Anything, job.SDRPath()).Return(nil).Once()

	stages := &recordedStages{}

	require.NoError(t, processJob(t.Context(), job, runner, nil, store, stages))

	assert.Equal(t, []clip.Stage{clip.StageSDR}, stages.reported())
	assert.FileExists(t, job.OutputPath)
	assert.FileExists(t, job.SDRPath(), "the SDR version is published beside the clip")

	var encodes [][]string

	for _, argv := range stubInvocations(t, filepath.Join(dir, "argv.log")) {
		if stubArgvContains(argv, "-crf") {
			encodes = append(encodes, argv)
		}
	}

	require.Len(t, encodes, 2, "the clip and its SDR version are encoded")
	assert.True(t, stubArgvContains(encodes[0], "libx265"), "the clip keeps HDR as HEVC")
	assert.True(t, stubArgvContains(encodes[1], "libx264"), "the SDR version is H.264")
	assert.True(t, stubArgvContains(encodes[1], "tonemap=tonemap=mobius"), "tone mapped")
	assertStagedFor(t, job.SDRPath(), stubOutputArg(encodes[1]))
}

// TestProcessJobKeepsTheClipWhenItsSDRVersionFails covers a failed SDR
// version: the clip still completes with its own file, and the half-made SDR
// version is removed so no card plays it.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobKeepsTheClipWhenItsSDRVersionFails(t *testing.T) {
	dir := t.TempDir()
	job, runner := hdrClipJob(t, dir)

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Put(mock.Anything, job.OutputPath).Return(nil).Once()
	store.EXPECT().Put(mock.Anything, job.SDRPath()).Return(errStoreFailed).Once()
	store.EXPECT().DeleteFile(job.SDRPath()).Return(nil).Once()

	require.NoError(t, processJob(t.Context(), job, runner, nil, store, &recordedStages{}))
}

// TestProcessJobStopsWhenTheJobStopsDuringItsSDRVersion covers a job stopped
// during the SDR version, by a cancel or a shutdown: the error reaches the
// queue, which settles the job itself, rather than the clip completing with
// no SDR version.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobStopsWhenTheJobStopsDuringItsSDRVersion(t *testing.T) {
	dir := t.TempDir()
	job, runner := hdrClipJob(t, dir)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Put(mock.Anything, job.OutputPath).Return(nil).Once()
	store.EXPECT().Put(mock.Anything, job.SDRPath()).
		RunAndReturn(func(context.Context, string) error {
			cancel()

			return context.Canceled
		}).Once()

	err := processJob(ctx, job, runner, nil, store, &recordedStages{})
	require.ErrorIs(t, err, context.Canceled)
	assert.ErrorContains(t, err, "sdr version")
}

// TestProcessJobKeepsTheSDRVersionWhenTheJobStopsBeforeItsProbe covers a job
// stopped between the clip and its SDR version: the probe that fails because
// of the stop is not read as an SDR source, so the stored SDR version stays
// and the stop reaches the queue.
//
//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobKeepsTheSDRVersionWhenTheJobStopsBeforeItsProbe(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("stopped-before-probe")

	job.PreserveHDR = true
	job.InputPath = stubInputFile(t, dir, "stopped.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	// No ffprobe, so only the stop decides what the missing probe means.
	runner := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), ffmpegtest.Stub{}),
		missingBinary(dir),
	)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	store := storagemocks.NewMockBlob(t)
	store.EXPECT().Put(mock.Anything, job.OutputPath).
		RunAndReturn(func(context.Context, string) error {
			cancel()

			return nil
		}).Once()

	err := processJob(ctx, job, runner, nil, store, &recordedStages{})
	require.ErrorIs(t, err, context.Canceled)
}
