// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package app

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	storagemocks "github.com/PapagoLabs/outtake/internal/store/blob/mocks"
)

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
		WebSafeColor:  false,
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

	err := extractJob(t.Context(), job, nil, nil)
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

	execFFmpeg := ffmpeg.NewExecFFmpeg(stubFFmpeg(t, logPath, "exit 0\n"), missingBinary(dir))

	require.NoError(t, extractJob(t.Context(), job, execFFmpeg, nil))

	invocations := stubInvocations(t, logPath)
	require.Len(t, invocations, 1, "a clip without crop trimming runs one pass")
	assert.Equal(t, job.OutputPath, stubOutputArg(invocations[0]))
	assert.Contains(t, invocations[0], "-crf", "the clip arm renders an H.264 stream")
	assert.Contains(t, invocations[0], "libx264")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobRendersAClipWithADetectedCrop(t *testing.T) {
	// A stub body whose stderr carries a cropdetect result.
	const cropdetectSucceeds = "cat >&2 <<'STUB_ERR'\n" +
		"Stream #0:0: Video: hevc, yuv420p, 1920x1080\n" +
		"[Parsed_cropdetect_0 @ 0x1] x1:0 x2:1919 y1:140 y2:939 w:1920 h:800 crop=1920:800:0:140\n" +
		"STUB_ERR\n" +
		"exit 0\n"

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

	require.NoError(t, extractJob(t.Context(), job, execFFmpeg, nil))

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

	assert.Equal(t, job.OutputPath, stubOutputArg(encodeArgv))
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestExtractJobReportsAClipEncodeFailure(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("clip-failure")

	job.InputPath = stubInputFile(t, dir, "failure.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), "exit 1\n"),
		missingBinary(dir),
	)

	err := extractJob(t.Context(), job, execFFmpeg, nil)
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

	execFFmpeg := ffmpeg.NewExecFFmpeg(stubFFmpeg(t, logPath, "exit 0\n"), missingBinary(dir))

	require.NoError(t, extractJob(t.Context(), job, execFFmpeg, nil))

	invocations := stubInvocations(t, logPath)
	require.Len(t, invocations, 2, "a GIF runs a palette pass and an encode pass")

	assert.Equal(t, job.OutputPath+".palette.png", stubOutputArg(invocations[0]))
	assert.True(t, stubArgvContains(invocations[0], "palettegen=stats_mode=diff"))

	assert.Equal(t, job.OutputPath, stubOutputArg(invocations[1]))
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
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), "exit 1\n"),
		missingBinary(dir),
	)

	err := extractJob(t.Context(), job, execFFmpeg, nil)
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

	execFFmpeg := ffmpeg.NewExecFFmpeg(stubFFmpeg(t, logPath, "exit 0\n"), missingBinary(dir))

	require.NoError(t, extractJob(t.Context(), job, execFFmpeg, nil))

	invocations := stubInvocations(t, logPath)
	require.Len(t, invocations, 1, "a screenshot needs no second pass")
	assert.Equal(t, job.OutputPath, stubOutputArg(invocations[0]))
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
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), "exit 1\n"),
		missingBinary(dir),
	)

	err := extractJob(t.Context(), job, execFFmpeg, nil)
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

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), "exit 0\n"),
		missingBinary(dir),
	)

	require.NoError(t, processJob(t.Context(), job, execFFmpeg, nil, store))
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
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), "exit 0\n"),
		missingBinary(dir),
	)

	err := processJob(t.Context(), job, execFFmpeg, nil, store)
	require.ErrorIs(t, err, errStoreFailed)
	assert.ErrorContains(t, err, "store output")
}

//nolint:paralleltest // The render reads the process-global logger New rewrites.
func TestProcessJobSkipsTheUploadWithoutAnOutput(t *testing.T) {
	dir := t.TempDir()

	job := testClipJob("no-output")

	job.InputPath = stubInputFile(t, dir, "no-output.mkv")

	store := storagemocks.NewMockBlob(t)

	execFFmpeg := ffmpeg.NewExecFFmpeg(
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), "exit 0\n"),
		missingBinary(dir),
	)

	require.NoError(t, processJob(t.Context(), job, execFFmpeg, nil, store),
		"nothing was rendered to a path, so there is nothing to upload")
}

func TestProcessJobReportsAnExtractFailureBeforeUploading(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	job := testClipJob("extract-failure")

	job.Type = clip.Type("video")
	job.InputPath = stubInputFile(t, dir, "never-rendered.mkv")
	job.OutputPath = filepath.Join(dir, "clip.mp4")

	store := storagemocks.NewMockBlob(t)

	err := processJob(t.Context(), job, nil, nil, store)
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
	execFFmpeg := ffmpeg.NewExecFFmpeg(stubFFmpeg(t, logPath, "exit 0\n"), missingBinary(dir))

	rect := detectJobCrop(t.Context(), execFFmpeg, job)

	assert.Equal(t, crop.CropRect{}, rect)
	assert.Empty(t, stubInvocations(t, logPath), "no detection pass was run at all")
}

//nolint:paralleltest // The detection pass reads the process-global logger New rewrites.
func TestDetectJobCropReturnsTheDetectedRectangle(t *testing.T) {
	// A stub body whose stderr carries a cropdetect result.
	const cropdetectSucceeds = "cat >&2 <<'STUB_ERR'\n" +
		"Stream #0:0: Video: hevc, yuv420p, 1920x1080\n" +
		"[Parsed_cropdetect_0 @ 0x1] x1:0 x2:1919 y1:140 y2:939 w:1920 h:800 crop=1920:800:0:140\n" +
		"STUB_ERR\n" +
		"exit 0\n"

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
		stubFFmpeg(t, filepath.Join(dir, "argv.log"), "exit 1\n"),
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
