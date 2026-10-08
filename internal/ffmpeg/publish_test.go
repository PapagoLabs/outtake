// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
)

// renderClip runs a clip render with an ffmpeg fake over an existing output.
//
// Parameters:
//   - t: The test the render belongs to.
//   - stub: What the fake ffmpeg does.
//
// Returns:
//   - output: The output path, which held "good" before the render.
//   - err: What the render returned.
func renderClip(t *testing.T, stub ffmpegtest.Stub) (string, error) {
	t.Helper()

	fixture := newEncodeFixture(t, "clip.mp4")
	require.NoError(t, os.WriteFile(fixture.output, []byte("good"), 0o600))

	err := NewExecFFmpeg(ffmpegtest.Install(t, stub), "unused").ExtractClip(
		t.Context(),
		fixture.input,
		fixture.output,
		time.Second,
		5*time.Second,
		clip.QualityPresets[clip.ClipQualityMedium],
		0,
		crop.CropRect{},
	)

	//nolint:wrapcheck // The test reads the render's own error.
	return fixture.output, err
}

// stagedFiles lists the staging files and palettes left in a directory.
//
// Parameters:
//   - t: The test that is looking.
//   - dir: Directory to read.
//
// Returns:
//   - names: The leftover file names.
func stagedFiles(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var names []string

	for _, entry := range entries {
		if !entry.IsDir() && isLeftover(entry.Name()) && !strings.HasSuffix(entry.Name(), ".argv") {
			names = append(names, entry.Name())
		}
	}

	return names
}

// TestAFailedRenderKeepsTheFileItWasReplacing covers a regenerate that fails
// part way: the clip already on disk survives, and nothing is left beside it.
func TestAFailedRenderKeepsTheFileItWasReplacing(t *testing.T) {
	t.Parallel()

	output, err := renderClip(t, ffmpegtest.Stub{Output: "partial", ExitCode: 1})
	require.Error(t, err)

	kept, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	assert.Equal(t, "good", string(kept), "the earlier render is untouched")
	assert.Empty(t, stagedFiles(t, filepath.Dir(output)), "the partial file is removed")
}

// TestARenderThatWritesNothingFails covers an encode that exits cleanly with
// an empty file, or with none at all.
func TestARenderThatWritesNothingFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		stub ffmpegtest.Stub
	}{
		{name: "no file", stub: ffmpegtest.Stub{}},
		{name: "an empty file", stub: ffmpegtest.Stub{Output: "", Stdout: ""}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			output, err := renderClip(t, test.stub)
			require.ErrorIs(t, err, ErrEmptyOutput)

			kept, readErr := os.ReadFile(output)
			require.NoError(t, readErr)
			assert.Equal(t, "good", string(kept))
		})
	}
}

// TestAnEmptyStagedFileFails covers the size check itself, which the ffmpeg
// fake cannot reach because it writes only non-empty output.
func TestAnEmptyStagedFileFails(t *testing.T) {
	t.Parallel()

	output := filepath.Join(t.TempDir(), "clip.mp4")

	err := publish(t.Context(), output, func(staging string) error {
		return os.WriteFile(staging, nil, 0o600)
	})
	require.ErrorIs(t, err, ErrEmptyOutput)

	assert.NoFileExists(t, output)
	assert.Empty(t, stagedFiles(t, filepath.Dir(output)))
}

// TestASuccessfulRenderReplacesTheFile covers a regenerate that finishes.
func TestASuccessfulRenderReplacesTheFile(t *testing.T) {
	t.Parallel()

	output, err := renderClip(t, ffmpegtest.Stub{Output: "fresh"})
	require.NoError(t, err)

	published, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	assert.Equal(t, "fresh", string(published))
	assert.Empty(t, stagedFiles(t, filepath.Dir(output)))
}

// TestStagingPathKeepsTheExtension covers the staging name: hidden, beside
// the output, and ending as the output does, which ffmpeg picks the container
// from.
func TestStagingPathKeepsTheExtension(t *testing.T) {
	t.Parallel()

	staging := stagingPath("/out/clips/abc.gif")

	assert.Equal(t, "/out/clips", filepath.Dir(staging))
	assert.True(t, strings.HasPrefix(filepath.Base(staging), stagingPrefix))
	assert.Equal(t, ".gif", filepath.Ext(staging))
	assert.NotEqual(
		t,
		staging,
		stagingPath("/out/clips/abc.gif"),
		"each render stages its own file",
	)
}

// TestSweepStagedRemovesOnlyLeftovers covers the startup sweep.
func TestSweepStagedRemovesOnlyLeftovers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	old := time.Now().Add(-2 * time.Hour)

	for _, name := range []string{
		".staging-a.mp4",
		".staging-b.gif.palette.png",
		"c.gif.palette.png",
		"kept.mp4",
		"kept.palette.png",
	} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
		require.NoError(t, os.Chtimes(path, old, old))
	}

	// Another process sharing the directory is still writing this one.
	live := filepath.Join(dir, ".staging-live.mp4")
	require.NoError(t, os.WriteFile(live, []byte("x"), 0o600))

	require.NoError(t, os.Mkdir(filepath.Join(dir, ".staging-dir"), 0o750))

	removed := SweepStaged(time.Now().Add(-time.Hour), dir, filepath.Join(dir, "missing"))

	assert.Equal(t, 3, removed)
	assert.FileExists(t, live, "a staging file written since the cutoff belongs to a live render")
	require.NoError(t, os.Remove(live))
	assert.FileExists(t, filepath.Join(dir, "kept.mp4"))
	assert.FileExists(t, filepath.Join(dir, "kept.palette.png"),
		"a PNG is only a leftover palette when it was written beside a GIF")
	assert.DirExists(t, filepath.Join(dir, ".staging-dir"), "only files are swept")
	assert.Empty(t, stagedFiles(t, dir))
}

// TestExtractGIFPublishesAndLeavesNoPalette covers the two-pass GIF: the
// palette lives beside the staging file and goes with it.
func TestExtractGIFPublishesAndLeavesNoPalette(t *testing.T) {
	t.Parallel()

	fixture := newEncodeFixture(t, "clip.gif")

	err := NewExecFFmpeg(
		ffmpegtest.Install(t, ffmpegtest.Stub{Output: "gif"}),
		"unused",
	).ExtractGIF(
		t.Context(),
		fixture.input, fixture.output, time.Second, 3*time.Second, 0, 0, crop.CropRect{},
	)
	require.NoError(t, err)

	assert.FileExists(t, fixture.output)
	assert.Empty(t, stagedFiles(t, filepath.Dir(fixture.output)))
}

// TestEncodesAbortOnEmptyOutputAndDropSourceMetadata covers the flags every
// clip encode carries.
func TestEncodesAbortOnEmptyOutputAndDropSourceMetadata(t *testing.T) {
	t.Parallel()

	args := recordClipEncode(
		t,
		sdrEncodeExec(t),
		clip.QualityPresets[clip.ClipQualityHigh],
		crop.CropRect{},
	)

	// The recorded argv starts after the binary.
	assert.Equal(t, []string{abortOnFlag, abortOnEmptyOutput}, args[1:3],
		"an encode with no frames fails instead of finishing empty")
	assert.Subset(t, args, []string{mapMetadataFlag, mapChaptersFlag})
	assert.Equal(
		t,
		dropAll,
		args[indexOf(args, mapMetadataFlag)+1],
		"the source's title and tags are dropped",
	)
	assert.Equal(t, dropAll, args[indexOf(args, mapChaptersFlag)+1], "and so are its chapters")

	still := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", time.Second, crop.CropRect{}, "")
	assert.Equal(t, []string{abortOnFlag, abortOnEmptyOutput}, still[2:4])

	gif := gifSeekArgs("ffmpeg", "/in.mkv", time.Second, time.Second)
	assert.Equal(t, []string{abortOnFlag, abortOnEmptyOutput}, gif[2:4])
}

// indexOf returns where a value first appears in args.
//
// Parameters:
//   - args: The argv to search.
//   - value: The argument to find.
//
// Returns:
//   - index: Its position, or -1 when absent.
func indexOf(args []string, value string) int {
	for index, arg := range args {
		if arg == value {
			return index
		}
	}

	return -1
}

// TestACanceledRenderIsNotPublished covers a cancel that lands after ffmpeg
// finished: the staged file is discarded and the existing output is kept.
func TestACanceledRenderIsNotPublished(t *testing.T) {
	t.Parallel()

	output := filepath.Join(t.TempDir(), "clip.mp4")
	require.NoError(t, os.WriteFile(output, []byte("good"), 0o600))

	ctx, cancel := context.WithCancel(t.Context())

	err := publish(ctx, output, func(staging string) error {
		cancel()

		return os.WriteFile(staging, []byte("fresh"), 0o600)
	})
	require.ErrorIs(t, err, context.Canceled)

	kept, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	assert.Equal(t, "good", string(kept))
	assert.Empty(t, stagedFiles(t, filepath.Dir(output)))
}

// TestAClipCarriesNoSourceMetadata encodes a tagged source with the real
// ffmpeg and checks that neither its global nor its per-stream tags reach the
// clip.
func TestAClipCarriesNoSourceMetadata(t *testing.T) {
	t.Parallel()

	ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobePath, ffprobeErr := exec.LookPath("ffprobe")

	if ffmpegErr != nil || ffprobeErr != nil {
		t.Skip("ffmpeg and ffprobe are not available")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")

	makeSource := exec.CommandContext(
		t.Context(),
		ffmpegPath,
		"-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=24:duration=1",
		"-f", "lavfi", "-i", "sine=duration=1",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-metadata", "title=SourceTitle",
		"-metadata:s:v:0", "title=VideoTag",
		"-metadata:s:a:0", "title=AudioTag",
		"-metadata:s:a:0", "language=fre",
		src,
	)
	require.NoError(t, makeSource.Run())

	out := filepath.Join(dir, "clip.mp4")

	err := NewExecFFmpeg(ffmpegPath, ffprobePath).ExtractClip(
		t.Context(), src, out, 0, 500*time.Millisecond,
		clip.QualityPresets[clip.ClipQualityLow], 0, crop.CropRect{},
	)
	require.NoError(t, err)

	tags, err := exec.CommandContext(
		t.Context(),
		ffprobePath,
		"-v", "error",
		"-show_entries", "format_tags:stream_tags",
		"-of", "compact",
		out,
	).Output()
	require.NoError(t, err)

	for _, tag := range []string{"SourceTitle", "VideoTag", "AudioTag", "language=fre"} {
		assert.NotContains(t, string(tags), tag)
	}
}

// toneMapFFmpeg returns the real ffmpeg and ffprobe for the tone map test,
// skipping the test when either is missing, when ffmpeg has no libx265 to
// build the HDR10 source with, or when ffmpeg is older than 8.0, which copies
// a source's container HDR10 metadata into every output.
//
// Parameters:
//   - t: The test that needs the tools.
//
// Returns:
//   - ffmpegPath: The ffmpeg binary.
//   - ffprobePath: The ffprobe binary.
func toneMapFFmpeg(t *testing.T) (string, string) {
	t.Helper()

	ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobePath, ffprobeErr := exec.LookPath("ffprobe")

	if ffmpegErr != nil || ffprobeErr != nil {
		t.Skip("ffmpeg and ffprobe are not available")
	}

	encoders, err := exec.CommandContext(t.Context(), ffmpegPath, "-hide_banner", "-encoders").
		Output()
	if err != nil || !strings.Contains(string(encoders), "libx265") {
		t.Skip("ffmpeg has no libx265")
	}

	version, err := exec.CommandContext(t.Context(), ffmpegPath, "-version").Output()
	require.NoError(t, err)

	match := regexp.MustCompile(`version n?(\d+)\.`).FindStringSubmatch(string(version))
	if len(match) < 2 {
		t.Skip("the ffmpeg version cannot be read")
	}

	major, err := strconv.Atoi(match[1])
	if err != nil || major < 8 {
		t.Skip("ffmpeg before 8.0 copies container HDR10 metadata into every output")
	}

	return ffmpegPath, ffprobePath
}

// TestAToneMappedClipIsTaggedBT709AndCarriesNoHDRMetadata encodes a PQ source
// whose container carries HDR10 light levels and checks that the tone mapped
// clip is tagged BT.709 and carries no mastering display or light level data,
// which would mislabel an SDR file as HDR.
func TestAToneMappedClipIsTaggedBT709AndCarriesNoHDRMetadata(t *testing.T) {
	t.Parallel()

	ffmpegPath, ffprobePath := toneMapFFmpeg(t)

	dir := t.TempDir()
	seiOnly := filepath.Join(dir, "sei.mkv")
	src := filepath.Join(dir, "src.mkv")

	makeSEI := exec.CommandContext(t.Context(), ffmpegPath, "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=1",
		"-vf", "format=gbrp,zscale=tin=iec61966-2-1:pin=bt709:rin=full:"+
			"t=smpte2084:p=bt2020:m=bt2020nc:r=tv:npl=203,format=yuv420p10le",
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
		"-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:"+
			"master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):"+
			"max-cll=1000,400",
		seiOnly,
	)
	require.NoError(t, makeSEI.Run())

	// A second pass moves the light levels into the container as well, as
	// FFmpeg does for a source whose frames carry them.
	makeSource := exec.CommandContext(t.Context(), ffmpegPath, "-y", "-i", seiOnly,
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-x265-params", "log-level=error", src)
	require.NoError(t, makeSource.Run())

	sourceInfo, err := exec.CommandContext(t.Context(), ffprobePath, "-v", "error",
		"-show_streams", "-of", "json", src).Output()
	require.NoError(t, err)
	require.Contains(t, string(sourceInfo), "Content light level metadata",
		"the source must carry the light levels in its container for this test to mean anything")

	out := filepath.Join(dir, "clip.mp4")

	err = NewExecFFmpeg(ffmpegPath, ffprobePath).ExtractClip(
		t.Context(), src, out, 0, 500*time.Millisecond,
		clip.QualityPresets[clip.ClipQualityLow], 0, crop.CropRect{},
	)
	require.NoError(t, err)

	streams, err := exec.CommandContext(t.Context(), ffprobePath, "-v", "error",
		"-select_streams", "v:0", "-show_streams", "-show_frames", "-read_intervals", "%+#1",
		"-of", "json", out).Output()
	require.NoError(t, err)

	body := string(streams)
	assert.Contains(t, body, `"color_transfer": "bt709"`)
	assert.Contains(t, body, `"color_primaries": "bt709"`)
	assert.NotContains(t, body, "Mastering display metadata")
	assert.NotContains(t, body, "Content light level metadata")
}
