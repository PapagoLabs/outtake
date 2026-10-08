// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/tonemap"
)

func recordClipEncode(
	t *testing.T,
	execFFmpeg *ExecFFmpeg,
	preset clip.QualityPreset,
	rect crop.CropRect,
) []string {
	t.Helper()

	fixture := newEncodeFixture(t, "clip.mp4")

	err := execFFmpeg.ExtractClip(
		t.Context(),
		fixture.input,
		fixture.output,
		10*time.Second,
		5*time.Second,
		preset,
		1,
		rect,
	)
	require.NoError(t, err)

	return readRecordedArgv(t, fixture.output)
}

func recordPreviewEncode(
	t *testing.T,
	execFFmpeg *ExecFFmpeg,
	preset clip.QualityPreset,
	rect crop.CropRect,
) []string {
	t.Helper()

	fixture := newEncodeFixture(t, "preview.mp4")

	err := execFFmpeg.ExtractPreview(
		t.Context(),
		fixture.input,
		fixture.output,
		10*time.Second,
		5*time.Second,
		1,
		rect,
		preset,
	)
	require.NoError(t, err)

	return readRecordedArgv(t, fixture.output)
}

func TestExecFFmpeg_ExtractClip_MissingInput(t *testing.T) {
	t.Parallel()

	ff := NewExecFFmpeg("ffmpeg", "ffprobe")
	err := ff.ExtractClip(
		t.Context(),
		"/nonexistent/input.mp4",
		"/tmp/output.mp4",
		0,
		10,
		clip.QualityPresets[clip.ClipQualityMedium],
		0,
		crop.CropRect{},
	)
	assert.Error(t, err)
}

func TestExecFFmpeg_ExtractClip_Args(t *testing.T) {
	t.Parallel()

	fixture := newEncodeFixture(t, "output.mp4")
	ff := NewExecFFmpeg(ffmpegtest.Install(t, ffmpegtest.Stub{Output: "rendered"}), "unused")

	err := ff.ExtractClip(
		t.Context(),
		fixture.input,
		fixture.output,
		10500*time.Millisecond,
		30*time.Second,
		clip.QualityPresets[clip.ClipQualityHigh],
		1,
		crop.CropRect{},
	)
	require.NoError(t, err)

	published, err := os.ReadFile(fixture.output)
	require.NoError(t, err, "the staged render is moved into place")
	assert.Equal(t, "rendered", string(published))
}

func TestExtractClipEncodeArgs(t *testing.T) {
	t.Parallel()

	args := recordClipEncode(
		t,
		sdrEncodeExec(t),
		clip.QualityPresets[clip.ClipQualityHigh],
		crop.CropRect{},
	)

	assert.Contains(t, args, "-ac")
	assert.Contains(t, args, "2")
	assert.Contains(t, args, "-b:a")
	assert.Contains(t, args, "320k")
	assert.Contains(t, args, "0:a:1?")
	assert.Contains(t, args, "-pix_fmt")
	assert.Contains(t, args, pixelFormatYUV420P)
	assert.Contains(t, args, "-vf")
	assert.Contains(t, args, scaleFilter(clip.OutputWidth2160p, scaleFlagsLanczos))
	assert.NotContains(t, args, "128k")
	assert.NotContains(t, args, "crop=")
	assert.NotContains(t, args, "libplacebo")
	assert.NotContains(t, args, "zscale=tin=smpte2084")
	assert.NotContains(t, args, "write_colr")
}

func TestExtractClipEncodeArgsCropsBlackBars(t *testing.T) {
	t.Parallel()

	rect := crop.CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	args := recordClipEncode(t, sdrEncodeExec(t), clip.QualityPresets[clip.ClipQualityHigh], rect)

	assert.Contains(t, args,
		rect.Filter()+","+scaleFilter(clip.OutputWidth2160p, scaleFlagsLanczos))
}

// TestExtractClipEncodeArgsToneMapWithoutKeepHDR covers a video clip of an
// HDR source that does not keep HDR: it is tone mapped to Rec.709.
func TestExtractClipEncodeArgsToneMapWithoutKeepHDR(t *testing.T) {
	t.Parallel()

	preset := clip.QualityPresets[clip.ClipQualityHigh]

	preset.PreserveHDR = false

	args := recordClipEncode(t, hdrEncodeExec(t), preset, crop.CropRect{})
	joined := strings.Join(args, " ")
	wantFilter := tonemap.ToneMapFilter(
		clip.TransferPQAlias,
		tonemap.DefaultWebSafePeak,
		tonemap.TransferBT709,
	) +
		"," + scaleFilter(
		clip.OutputWidth2160p,
		scaleFlagsLanczos,
	)

	assert.Contains(t, args, wantFilter)
	assert.Contains(t, joined, "zscale=tin=smpte2084")
	assert.Contains(t, joined, "tonemap=tonemap=mobius")
	assert.Contains(t, args, "-color_primaries")
	assert.Contains(t, args, nameBT709)
	assert.Contains(t, args, "-color_trc")
	assert.Contains(t, args, tonemap.TransferBT709)
	assert.NotContains(t, args, tonemap.TransferSRGB, "SDR video is tagged BT.709, not sRGB")
	assert.Contains(t, args, toneMappedMovFlags)
	assert.NotContains(t, joined, "libplacebo")
	assert.NotContains(t, args, "-init_hw_device")
}

func TestExtractPreviewEncodeArgs(t *testing.T) {
	t.Parallel()

	args := recordPreviewEncode(t, sdrEncodeExec(t), clip.QualityPreset{}, crop.CropRect{})

	assert.Contains(t, args, "-pix_fmt")
	assert.Contains(t, args, pixelFormatYUV420P)
	assert.Contains(t, args, "-preset")
	assert.Contains(t, args, previewPreset)
	assert.Contains(t, args, "-crf")
	assert.Contains(t, args, strconv.Itoa(previewCRF))
	assert.Contains(t, args, strconv.Itoa(previewAudioKbps)+"k")
	assert.Contains(t, args, scaleFilter(previewMaxWidth, scaleFlagsFast))
	assert.NotContains(t, args, "320k")
	assert.NotContains(t, strings.Join(args, " "), "tonemap=tonemap=mobius")
}

// TestExtractPreviewEncodeArgsToneMapWithoutKeepHDR covers a preview of a
// clip that does not keep HDR: it is tone mapped like the clip.
func TestExtractPreviewEncodeArgsToneMapWithoutKeepHDR(t *testing.T) {
	t.Parallel()

	args := recordPreviewEncode(
		t,
		hdrEncodeExec(t),
		clip.QualityPreset{PreserveHDR: false},
		crop.CropRect{},
	)
	joined := strings.Join(args, " ")
	wantFilter := tonemap.ToneMapFilter(
		clip.TransferPQAlias,
		tonemap.DefaultWebSafePeak,
		tonemap.TransferBT709,
	) +
		"," + scaleFilter(
		previewMaxWidth,
		scaleFlagsFast,
	)

	assert.Contains(t, args, wantFilter)
	assert.Contains(t, joined, "tonemap=tonemap=mobius")
	assert.Contains(t, args, "-color_primaries")
	assert.Contains(t, args, nameBT709)
	assert.Contains(t, args, "-color_trc")
	assert.Contains(t, args, tonemap.TransferBT709)
	assert.NotContains(t, args, tonemap.TransferSRGB, "SDR video is tagged BT.709, not sRGB")
	assert.Contains(t, args, toneMappedMovFlags)
	assert.Contains(t, joined, scaleFilter(previewMaxWidth, scaleFlagsFast))
	assert.NotContains(t, joined, "libplacebo")
}

func TestPreviewDuration(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 10*time.Second, PreviewDuration(10*time.Second))
	assert.Equal(t, clip.MaxDuration, PreviewDuration(10*time.Minute))
	assert.Equal(t, time.Duration(0), PreviewDuration(0))
}

func TestScaleFilterForcesEvenWidth(t *testing.T) {
	t.Parallel()

	assert.Contains(t, scaleFilter(1920, scaleFlagsLanczos), "trunc(min(1920,iw)/2)*2")
	assert.Contains(t, scaleFilter(1920, scaleFlagsLanczos), "h=-2")
}
