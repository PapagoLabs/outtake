// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecFFmpeg_Run_CommandNotFound(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	err := ff.run(t.Context(), 0, "/nonexistent/ffmpeg", "-version")
	assert.Error(t, err)
}

func TestExecFFmpeg_Run_Timeout(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}

	ff := NewExecFFmpeg("sleep", "sleep")
	ff.SetTimeout(100 * time.Millisecond)

	err = ff.run(t.Context(), 0, "sleep", "10")
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractClip_Args(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("echo", "echo")
	ctx := t.Context()

	err := ff.ExtractClip(
		ctx,
		"/tmp/input.mp4",
		"/tmp/output.mp4",
		10.5,
		30.0,
		QualityPresets[ClipQualityHigh],
		1,
		CropRect{},
	)
	require.NoError(t, err)
}

func TestClipEncodeArgs(t *testing.T) {
	t.Parallel()

	args := clipEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		QualityPresets[ClipQualityHigh],
		1,
		CropRect{},
	)

	assert.Contains(t, args, "-ac")
	assert.Contains(t, args, "2")
	assert.Contains(t, args, "-b:a")
	assert.Contains(t, args, "320k")
	assert.Contains(t, args, "0:a:1?")
	assert.Contains(t, args, "-pix_fmt")
	assert.Contains(t, args, pixelFormatYUV420P)
	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, scaleFilter(OutputWidth2160p, scaleFlagsLanczos))
	assert.NotContains(t, args, "128k")
	assert.NotContains(t, args, "crop=")
	assert.NotContains(t, args, "libplacebo")
	assert.NotContains(t, args, "zscale=tin=smpte2084")
	assert.NotContains(t, args, "write_colr")
}

func TestClipEncodeArgsCropsBlackBars(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	args := clipEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		QualityPresets[ClipQualityHigh],
		1,
		crop,
	)

	assert.Contains(t, args, crop.Filter()+","+scaleFilter(OutputWidth2160p, scaleFlagsLanczos))
}

func TestClipEncodeArgsWebSafeColor(t *testing.T) {
	t.Parallel()

	req := clipEncodeRequest(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		QualityPresets[ClipQualityHigh],
		1,
		CropRect{},
	)
	applyColorPlan(&req, transferPQ, webSafeRemap)

	args := h264EncodeArgs(&req)
	joined := strings.Join(args, " ")
	wantFilter := webSafeToneMapFilter(transferPQAlias, defaultWebSafePeak) +
		"," + scaleFilter(OutputWidth2160p, scaleFlagsLanczos)
	assert.Contains(t, args, wantFilter)
	assert.Contains(t, joined, "zscale=tin=smpte2084")
	assert.Contains(t, joined, "tonemap=tonemap=hable")
	assert.Contains(t, args, "-color_primaries")
	assert.Contains(t, args, transferRec709Probe)
	assert.Contains(t, args, "-color_trc")
	assert.Contains(t, args, "iec61966-2-1")
	assert.Contains(t, args, webSafeMovFlags)
	assert.NotContains(t, joined, "libplacebo")
	assert.NotContains(t, args, "-init_hw_device")
}

func TestPreviewEncodeArgs(t *testing.T) {
	t.Parallel()

	args := previewEncodeArgs(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		1,
		CropRect{},
		QualityPreset{},
	)

	assert.Contains(t, args, "-pix_fmt")
	assert.Contains(t, args, pixelFormatYUV420P)
	assert.Contains(t, args, "-preset")
	assert.Contains(t, args, previewPreset)
	assert.Contains(t, args, "-crf")
	assert.Contains(t, args, strconv.Itoa(previewCRF))
	assert.Contains(t, args, strconv.Itoa(previewAudioKbps)+"k")
	assert.Contains(t, args, scaleFilter(previewMaxWidth, scaleFlagsFast))
	assert.NotContains(t, args, "320k")
	assert.NotContains(t, strings.Join(args, " "), "tonemap=tonemap=hable")
}

func TestPreviewEncodeArgsWebSafeColor(t *testing.T) {
	t.Parallel()

	req := previewEncodeRequest(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		1,
		CropRect{},
		QualityPreset{},
	)
	applyColorPlan(&req, transferPQ, webSafeRemap)

	args := h264EncodeArgs(&req)
	joined := strings.Join(args, " ")
	wantFilter := webSafeToneMapFilter(transferPQAlias, defaultWebSafePeak) +
		"," + scaleFilter(previewMaxWidth, scaleFlagsFast)
	assert.Contains(t, args, wantFilter)
	assert.Contains(t, joined, "tonemap=tonemap=hable")
	assert.Contains(t, args, "-color_primaries")
	assert.Contains(t, args, transferRec709Probe)
	assert.Contains(t, args, "-color_trc")
	assert.Contains(t, args, "iec61966-2-1")
	assert.Contains(t, args, webSafeMovFlags)
	assert.Contains(t, joined, scaleFilter(previewMaxWidth, scaleFlagsFast))
	assert.NotContains(t, joined, "libplacebo")
}

func TestPreviewDuration(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 10.0, PreviewDuration(10), 0.001)
	assert.InDelta(t, float64(previewMaxSecs), PreviewDuration(600), 0.001)
	assert.InDelta(t, 0.0, PreviewDuration(0), 0.001)
}

func TestScaleFilterForcesEvenWidth(t *testing.T) {
	t.Parallel()

	assert.Contains(t, scaleFilter(1920, scaleFlagsLanczos), "trunc(min(1920,iw)/2)*2")
	assert.Contains(t, scaleFilter(1920, scaleFlagsLanczos), "h=-2")
}

func TestExtractGIFWritesFile(t *testing.T) {
	t.Parallel()

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	makeCmd := exec.CommandContext(
		t.Context(),
		ffmpegPath,
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=size=320x240:rate=24:duration=1",
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		src,
	)
	require.NoError(t, makeCmd.Run())

	out := filepath.Join(dir, "out.gif")
	ff := NewExecFFmpeg(ffmpegPath, "ffprobe")
	ff.SetTimeout(30 * time.Second)

	err = ff.ExtractGIF(t.Context(), src, out, 0, 0.5, 160, 10, CropRect{})
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Greater(t, len(data), 6)
	assert.Equal(t, "GIF", string(data[:3]))
}

func TestExecFFmpeg_ExtractScreenshot_Args(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("echo", "echo")
	ctx := t.Context()

	err := ff.ExtractScreenshot(ctx, "/tmp/input.mp4", "/tmp/screenshot.jpg", 120.0, CropRect{})
	require.NoError(t, err)
}

func TestGIFPaletteFilterCropsBlackBars(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	want := crop.Filter() +
		",fps=10,scale=480:-2:flags=lanczos,format=yuv420p,palettegen=stats_mode=diff"

	assert.Equal(t, want, gifPaletteFilter(480, 10, crop))
}

func TestGIFEncodeFilterCropsBlackBars(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	want := "[0:v]" + crop.Filter() +
		",fps=10,scale=480:-2:flags=lanczos,format=yuv420p[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5"

	assert.Equal(t, want, gifEncodeFilter(480, 10, crop))
}

func TestGIFPaletteFilterOmitsCropWhenEmpty(t *testing.T) {
	t.Parallel()

	got := gifPaletteFilter(480, 10, CropRect{})
	want := "fps=10,scale=480:-2:flags=lanczos,format=yuv420p,palettegen=stats_mode=diff"

	assert.Equal(t, want, got)
	assert.NotContains(t, got, "crop=")
}

func TestGIFPaletteArgsLimitsInput(t *testing.T) {
	t.Parallel()

	args := gifPaletteArgs("ffmpeg", "/in.mkv", "/p.png", 10, 5, "vf")
	ss := slices.Index(args, ssFlag)
	dur := slices.Index(args, durationFlag)
	in := slices.Index(args, inputFlag)

	assert.Greater(t, dur, ss)
	assert.Greater(t, in, dur)
	assert.Contains(t, args, anFlag)
	assert.Contains(t, args, updateFlag)
	assert.Contains(t, args, framesFlag)
}

func TestGIFEncodeArgsLimitsInput(t *testing.T) {
	t.Parallel()

	args := gifEncodeArgs("ffmpeg", "/in.mkv", "/p.png", "/out.gif", 10, 5, "fc")
	firstInput := slices.Index(args, inputFlag)
	dur := slices.Index(args, durationFlag)

	assert.Greater(t, firstInput, dur)
	assert.Equal(t, "/in.mkv", args[firstInput+1])
	assert.Equal(t, "/p.png", args[firstInput+3])
	assert.Contains(t, args, anFlag)
	assert.NotContains(t, args[firstInput:], durationFlag)
}

func TestScreenshotEncodeArgsCropsBlackBars(t *testing.T) {
	t.Parallel()

	crop := CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	args := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", 12, crop)

	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, crop.Filter())
}

func TestScreenshotEncodeArgsOmitsCropWhenEmpty(t *testing.T) {
	t.Parallel()

	args := screenshotEncodeArgs("ffmpeg", "/in.mkv", "/out.jpg", 12, CropRect{})

	assert.NotContains(t, args, "-vf")
	assert.NotContains(t, args, "crop=")
}

// applyColorPlan resolves a request's color for a given source transfer, the
// part resolveColor does once the probe has answered.
//
// Parameters:
//   - req: Encode request to update.
//   - transfer: ffprobe's color transfer for the source.
//   - remap: How an HDR source should be handled.
func applyColorPlan(req *h264EncodeRequest, transfer string, remap remapDecision) {
	plan := decideColor(transfer, remap)

	req.hdrKind = plan.hdrKind
	req.toneMap = plan.toneMap
	req.colorTags = plan.colorTags
	req.pixFmt = plan.pixFmt
	if req.hdrKind == transferHLGAlias {
		req.tonePeak = defaultWebSafePeak
	}
}

// TestDecideColor pins how a source is tagged and whether it is remapped.
//
// The behavior under test is what a player sees. An SDR source and a preserved
// HDR source are both tagged, because an untagged file is the defect that
// started this: the player has to guess the transfer, and guesses differ.
func TestDecideColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		transfer      string
		remap         remapDecision
		wantHDRKind   string
		wantToneMap   bool
		wantTags      string
		wantNeedsPeak bool
		wantPixFmt    string
	}{
		{
			name:        "sdr is tagged rec709",
			transfer:    transferRec709Probe,
			remap:       webSafeRemap,
			wantHDRKind: "",
			wantTags:    TransferRec709,
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:          "pq is remapped by default and tagged rec709",
			transfer:      transferPQ,
			remap:         webSafeRemap,
			wantHDRKind:   transferPQAlias,
			wantToneMap:   true,
			wantTags:      TransferRec709,
			wantNeedsPeak: true,
			wantPixFmt:    pixelFormatYUV420P,
		},
		{
			name:        "pq is preserved, tagged, and kept 10-bit",
			transfer:    transferPQ,
			remap:       preserveHDR,
			wantHDRKind: transferPQAlias,
			wantTags:    transferPQ,
			wantPixFmt:  pixelFormatYUV420P10LE,
		},
		{
			name:        "hlg is remapped, tagged rec709, and needs no peak sample",
			transfer:    transferHLG,
			remap:       webSafeRemap,
			wantHDRKind: transferHLGAlias,
			wantToneMap: true,
			wantTags:    TransferRec709,
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:        "hlg is preserved, tagged, and kept 10-bit",
			transfer:    transferHLG,
			remap:       preserveHDR,
			wantHDRKind: transferHLGAlias,
			wantTags:    transferHLG,
			wantPixFmt:  pixelFormatYUV420P10LE,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan := decideColor(test.transfer, test.remap)

			assert.Equal(t, test.wantHDRKind, plan.hdrKind)
			assert.Equal(t, test.wantToneMap, plan.toneMap)
			assert.Equal(t, test.wantNeedsPeak, plan.needsPeak)
			assert.Equal(t, test.wantPixFmt, plan.pixFmt)
			assert.Contains(t, plan.colorTags, "-color_primaries")
			assert.Contains(t, plan.colorTags, test.wantTags,
				"an output must declare the transfer it carries")
		})
	}
}

// TestPreservedHDRIsTaggedForItsTransfer covers the case the branch exists for.
// A preserved PQ source is not remapped, but it is still described, so the file
// is not left for a player to guess at.
func TestPreservedHDRIsTaggedForItsTransfer(t *testing.T) {
	t.Parallel()

	req := clipEncodeRequest(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		QualityPresets[ClipQualityHigh],
		1,
		CropRect{},
	)
	applyColorPlan(&req, transferPQ, preserveHDR)

	args := h264EncodeArgs(&req)
	joined := strings.Join(args, " ")

	assert.Contains(t, args, primariesBT2020)
	assert.Contains(t, args, transferPQ)
	assert.NotContains(t, joined, "tonemap=tonemap=hable",
		"a preserved source must not be remapped")
	assert.NotContains(t, joined, "zscale=tin=smpte2084")
	assert.Contains(t, args, pixelFormatYUV420P10LE,
		"a preserved PQ source needs 10-bit, because 8-bit PQ bands")
}

// TestPreservedHDRStaysTenBit guards the bit depth that the pixel format
// carries. Tagging a file PQ while storing it in 8-bit leaves it banded, so the
// two have to move together.
func TestPreservedHDRStaysTenBit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		transfer string
		remap    remapDecision
		want     string
	}{
		{
			name:     "preserved pq is 10-bit",
			transfer: transferPQ,
			remap:    preserveHDR,
			want:     pixelFormatYUV420P10LE,
		},
		{
			name:     "preserved hlg is 10-bit",
			transfer: transferHLG,
			remap:    preserveHDR,
			want:     pixelFormatYUV420P10LE,
		},
		{
			name:     "remapped pq is 8-bit",
			transfer: transferPQ,
			remap:    webSafeRemap,
			want:     pixelFormatYUV420P,
		},
		{
			name:     "remapped hlg is 8-bit",
			transfer: transferHLG,
			remap:    webSafeRemap,
			want:     pixelFormatYUV420P,
		},
		{
			name:     "sdr is 8-bit",
			transfer: transferRec709Probe,
			remap:    preserveHDR,
			want:     pixelFormatYUV420P,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := clipEncodeRequest(
				"ffmpeg", "/in.mkv", "/out.mp4", 10, 5,
				QualityPresets[ClipQualityHigh], 1, CropRect{},
			)
			applyColorPlan(&req, test.transfer, test.remap)

			assert.Equal(t, test.want, pixelFormat(&req))
		})
	}
}
