// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/ffmpegtest"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/tonemap"
)

// hlgProbe is an ffprobe answer for an HLG source, which tone maps against a
// nominal peak and so needs no luma sampling pass.
const hlgProbe = `{"format":{"duration":"60.0","bit_rate":"8000","format_name":"matroska"},` +
	`"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","width":3840,` +
	`"height":2160,"color_transfer":"arib-std-b67"}]}`

// pqProbe is an ffprobe answer for a PQ source, whose peak is sampled.
const pqProbe = `{"format":{"duration":"60.0","bit_rate":"8000","format_name":"matroska"},` +
	`"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","width":3840,` +
	`"height":2160,"color_transfer":"smpte2084"}]}`

// sdrProbe is an ffprobe answer for a Rec.709 source.
const sdrProbe = `{"format":{"duration":"60.0","bit_rate":"8000","format_name":"matroska"},` +
	`"streams":[{"index":0,"codec_type":"video","codec_name":"h264","width":1920,` +
	`"height":1080,"color_transfer":"bt709"}]}`

// signalstatsLog is the stderr a stubbed luma pass writes.
const signalstatsLog = "lavfi.signalstats.YMAX=143.0\nlavfi.signalstats.YMAX=158.0\n"

func applyColorPlan(req *h264EncodeRequest, transfer string, remap remapDecision) {
	plan := decideColor(transfer, remap)

	req.hdrKind = plan.hdrKind
	req.toneMap = plan.toneMap
	req.colorTags = plan.colorTags
	req.pixFmt = plan.pixFmt
	if req.hdrKind == clip.TransferHLGAlias {
		req.tonePeak = tonemap.DefaultWebSafePeak
	}
}

// TestRemapDecisionFollowsKeepHDR covers the one switch: a clip that keeps
// HDR keeps the source transfer, and any other clip is tone mapped.
func TestRemapDecisionFollowsKeepHDR(t *testing.T) {
	t.Parallel()

	assert.Equal(t, keepHDR, remapDecisionFor(clip.QualityPreset{PreserveHDR: true}))
	assert.Equal(t, toneMapSDR, remapDecisionFor(clip.QualityPreset{PreserveHDR: false}))
}

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
			name:        "an sdr source keeps the transfer it was probed with",
			transfer:    nameBT709,
			remap:       toneMapSDR,
			wantHDRKind: "",
			wantTags:    nameBT709,
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:        "an sdr source with an unreported transfer is left untagged",
			transfer:    "unknown",
			remap:       toneMapSDR,
			wantHDRKind: "",
			wantTags:    "",
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:        "a pal sdr source is not claimed as srgb",
			transfer:    "bt470bg",
			remap:       toneMapSDR,
			wantHDRKind: "",
			wantTags:    "bt470bg",
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:          "pq is remapped by default and tagged rec709",
			transfer:      clip.TransferPQ,
			remap:         toneMapSDR,
			wantHDRKind:   clip.TransferPQAlias,
			wantToneMap:   true,
			wantTags:      tonemap.TransferBT709,
			wantNeedsPeak: true,
			wantPixFmt:    pixelFormatYUV420P,
		},
		{
			name:        "pq is preserved, tagged, and kept 10-bit",
			transfer:    clip.TransferPQ,
			remap:       keepHDR,
			wantHDRKind: clip.TransferPQAlias,
			wantTags:    clip.TransferPQ,
			wantPixFmt:  pixelFormatYUV420P10LE,
		},
		{
			name:        "hlg is remapped, tagged rec709, and needs no peak sample",
			transfer:    clip.TransferHLG,
			remap:       toneMapSDR,
			wantHDRKind: clip.TransferHLGAlias,
			wantToneMap: true,
			wantTags:    tonemap.TransferBT709,
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:        "hlg is preserved, tagged, and kept 10-bit",
			transfer:    clip.TransferHLG,
			remap:       keepHDR,
			wantHDRKind: clip.TransferHLGAlias,
			wantTags:    clip.TransferHLG,
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

			if test.wantTags == "" {
				assert.Empty(t, plan.colorTags,
					"an unreported transfer is left untagged rather than guessed")
			} else {
				assert.Contains(t, plan.colorTags, "-color_primaries")
				assert.Contains(t, plan.colorTags, test.wantTags,
					"an output must declare the transfer it carries")
			}
		})
	}
}

func TestPreservedHDRIsTaggedForItsTransfer(t *testing.T) {
	t.Parallel()

	req := clipEncodeRequest(
		"ffmpeg",
		"/in.mkv",
		"/out.mp4",
		10,
		5,
		clip.QualityPresets[clip.ClipQualityHigh],
		1,
		crop.CropRect{},
	)
	applyColorPlan(&req, clip.TransferPQ, keepHDR)

	args := h264EncodeArgs(&req)
	joined := strings.Join(args, " ")

	assert.Contains(t, args, primariesBT2020)
	assert.Contains(t, args, clip.TransferPQ)
	assert.NotContains(t, joined, "tonemap=tonemap=mobius",
		"a preserved source must not be remapped")
	assert.NotContains(t, joined, "zscale=tin=smpte2084")
	assert.Contains(t, args, pixelFormatYUV420P10LE,
		"a preserved PQ source needs 10-bit, because 8-bit PQ bands")
}

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
			transfer: clip.TransferPQ,
			remap:    keepHDR,
			want:     pixelFormatYUV420P10LE,
		},
		{
			name:     "preserved hlg is 10-bit",
			transfer: clip.TransferHLG,
			remap:    keepHDR,
			want:     pixelFormatYUV420P10LE,
		},
		{
			name:     "remapped pq is 8-bit",
			transfer: clip.TransferPQ,
			remap:    toneMapSDR,
			want:     pixelFormatYUV420P,
		},
		{
			name:     "remapped hlg is 8-bit",
			transfer: clip.TransferHLG,
			remap:    toneMapSDR,
			want:     pixelFormatYUV420P,
		},
		{
			name:     "sdr is 8-bit",
			transfer: nameBT709,
			remap:    keepHDR,
			want:     pixelFormatYUV420P,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := clipEncodeRequest(
				"ffmpeg", "/in.mkv", "/out.mp4", 10, 5,
				clip.QualityPresets[clip.ClipQualityHigh], 1, crop.CropRect{},
			)
			applyColorPlan(&req, test.transfer, test.remap)

			assert.Equal(t, test.want, pixelFormat(&req))
		})
	}
}

func peakCached(identity probe.Key) bool {
	_, found := identity.CachedPeak(10*time.Second, peakSampleSeconds(30*time.Second))

	return found
}

func TestSignalstatsCachesPeak(t *testing.T) {
	t.Parallel()

	path, identity := newPassSource(t)
	execFFmpeg := NewExecFFmpeg(passStub(t, signalstatsLog), "unused")

	first, ok := execFFmpeg.signalstatsYMax(t.Context(), path, 10*time.Second, 30*time.Second)
	require.True(t, ok, "the stubbed signalstats output must parse")
	assert.InDelta(t, 158.0, first, 0.001)

	assert.True(
		t,
		peakCached(identity),
		"a successful sample must be cached",
	)

	blind := blindExec()

	second, ok := blind.signalstatsYMax(t.Context(), path, 10*time.Second, 30*time.Second)
	require.True(t, ok)
	assert.InDelta(t, first, second, 0.0005)

	third, ok := blind.signalstatsYMax(t.Context(), path, 10*time.Second, 600*time.Second)
	require.True(t, ok, "a longer clip must reuse the clamped entry")
	assert.InDelta(t, first, third, 0.0005)
}

func TestSignalstatsDoesNotCacheFailedRun(t *testing.T) {
	t.Parallel()

	path, identity := newPassSource(t)
	execFFmpeg := NewExecFFmpeg(failingPassStub(t, signalstatsLog), "unused")

	_, ok := execFFmpeg.signalstatsYMax(t.Context(), path, 10*time.Second, 30*time.Second)
	assert.False(
		t,
		ok,
		"a luma value from a pass that errored may be a partial sample, so none is used",
	)

	assert.False(
		t,
		peakCached(identity),
		"a pass that errored must not be cached",
	)

	assert.InDelta(
		t,
		tonemap.DefaultWebSafePeak,
		execFFmpeg.tonePeak(
			t.Context(),
			path,
			clip.TransferPQAlias,
			10*time.Second,
			30*time.Second,
		),
		0.0001,
		"the tone map falls back to the nominal peak",
	)
}

func TestSignalstatsDoesNotCacheFailure(t *testing.T) {
	t.Parallel()

	path, identity := newPassSource(t)
	execFFmpeg := NewExecFFmpeg(
		passStub(t, "no signalstats output here"),
		"unused",
	)

	_, ok := execFFmpeg.signalstatsYMax(t.Context(), path, 10*time.Second, 30*time.Second)
	require.False(t, ok)

	assert.False(
		t,
		peakCached(identity),
		"a failed sample must not be cached",
	)
}

func TestSignalstatsSkipsCachingWhenFileChangesDuringProbe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "source.mkv")
	require.NoError(t, os.WriteFile(target, []byte("original-content"), 0o600))

	before, err := os.Stat(target)
	require.NoError(t, err)

	identity, ok := probe.KeyFor(target)
	require.True(t, ok)

	replacement := filepath.Join(dir, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replaced-content"), 0o600))
	require.NoError(t, os.Chtimes(replacement, before.ModTime(), before.ModTime()))

	execFFmpeg := NewExecFFmpeg(swappingPassStub(t, signalstatsLog), "unused")

	peak, ok := execFFmpeg.signalstatsYMax(t.Context(), target, 10*time.Second, 30*time.Second)
	require.True(t, ok, "the stub answers regardless of what the file contains")
	assert.InDelta(t, 158.0, peak, 0.001)

	assert.False(
		t,
		peakCached(identity),
		"a file that changed mid-pass must not be cached under the identity taken before it",
	)
}

func TestSignalstatsFilterSamplesEightBitLuma(t *testing.T) {
	t.Parallel()

	assert.True(
		t,
		strings.HasPrefix(signalstatsFilter, "format=yuv420p,"),
		"signalstats on a 10-bit source reports YMAX in 10-bit codes, and the peak math reads 8-bit",
	)
	assert.Contains(t, signalstatsFilter, "signalstats")
}

func TestPeakSampleSecondsPinsTheWindow(t *testing.T) {
	t.Parallel()

	assert.Equal(t, peakSampleCap, peakSampleSeconds(600*time.Second))
	assert.Equal(t, peakSampleCap, peakSampleSeconds(0))
	assert.Equal(t, 3500*time.Millisecond, peakSampleSeconds(3500*time.Millisecond))
}

// TestStillsAndGIFsAlwaysToneMapHDR covers the GIF and the screenshot:
// neither format can carry HDR, so an HDR source is always tone mapped, and an
// SDR source is left as it is.
func TestStillsAndGIFsAlwaysToneMapHDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		probe  string
		mapped bool
	}{
		{name: "HLG", probe: hlgProbe, mapped: true},
		{name: "PQ", probe: pqProbe, mapped: true},
		{name: "SDR", probe: sdrProbe, mapped: false},
	}

	exports := map[string]func(t *testing.T, runner *ExecFFmpeg, fixture encodeFixture){
		"gif": func(t *testing.T, runner *ExecFFmpeg, fixture encodeFixture) {
			t.Helper()

			require.NoError(t, runner.ExtractGIF(
				t.Context(), fixture.input, fixture.output, time.Second, 3*time.Second, 0, 0,
				crop.CropRect{},
			))
		},
		"screenshot": func(t *testing.T, runner *ExecFFmpeg, fixture encodeFixture) {
			t.Helper()

			require.NoError(t, runner.ExtractScreenshot(
				t.Context(), fixture.input, fixture.output, time.Second, crop.CropRect{},
			))
		},
	}

	for export, render := range exports {
		for _, test := range tests {
			t.Run(export+" "+test.name, func(t *testing.T) {
				t.Parallel()

				fixture := newEncodeFixture(t, "out."+export)
				logPath := filepath.Join(t.TempDir(), "argv")

				runner := NewExecFFmpeg(
					ffmpegtest.Install(t, ffmpegtest.Stub{ArgvFile: logPath, Output: "rendered"}),
					probeStub(t, test.probe),
				)

				render(t, runner, fixture)

				recorded, err := os.ReadFile(logPath)
				require.NoError(t, err)

				if test.mapped {
					assert.Contains(
						t,
						string(recorded),
						"tonemap=mobius",
						"the HDR frames are mapped to Rec.709",
					)

					return
				}

				assert.NotContains(
					t,
					string(recorded),
					"tonemap=",
					"the frames are left as they are",
				)
			})
		}
	}
}

// TestToneMapSitsBetweenCropAndScale covers the filter order for a GIF and a
// still: the crop sees source pixels, and the scale sees mapped ones.
func TestToneMapSitsBetweenCropAndScale(t *testing.T) {
	t.Parallel()

	rect := crop.CropRect{Width: 1920, Height: 804, X: 0, Y: 138}
	toneMap := tonemap.ToneMapFilter(clip.TransferHLGAlias, 0, tonemap.TransferSRGB)

	gif := gifPaletteFilter(gifFrames{width: 480, fps: 10, rect: rect, toneMap: toneMap})
	assert.True(t, strings.HasPrefix(gif, rect.Filter()+","+toneMap+",fps=10"), gif)

	assert.Equal(t, rect.Filter()+","+toneMap, screenshotFilter(rect, toneMap))
	assert.Equal(t, toneMap, screenshotFilter(crop.CropRect{}, toneMap))
	assert.Equal(t, rect.Filter(), screenshotFilter(rect, ""))
	assert.Empty(t, screenshotFilter(crop.CropRect{}, ""))
}

// TestAScreenshotSamplesItsPeakNearTheStill covers the PQ peak of a
// screenshot of an HDR source with no HDR10 light levels: it is sampled from a
// short window starting at the still, not the longer window a clip uses.
func TestAScreenshotSamplesItsPeakNearTheStill(t *testing.T) {
	t.Parallel()

	fixture := newEncodeFixture(t, "still.jpg")
	logPath := filepath.Join(t.TempDir(), "argv")

	runner := NewExecFFmpeg(
		ffmpegtest.Install(t, ffmpegtest.Stub{
			ArgvFile:      logPath,
			ArgvSeparator: "--",
			Output:        "frame",
		}),
		probeStub(t, pqProbe),
	)

	require.NoError(t, runner.ExtractScreenshot(
		t.Context(), fixture.input, fixture.output, 90*time.Second, crop.CropRect{},
	))

	recorded, err := os.ReadFile(logPath)
	require.NoError(t, err)

	passes := strings.Split(string(recorded), "--\n")
	require.GreaterOrEqual(t, len(passes), 2, "a luma pass runs before the still is encoded")

	sample := strings.Fields(passes[0])
	require.Contains(t, sample, signalstatsFilter, "the first pass samples luma")
	assert.Equal(t, "90.000", sample[indexOf(sample, ssFlag)+1], "it starts at the still")
	assert.Equal(t, "1.000", sample[indexOf(sample, durationFlag)+1], "and covers one second")
}

// pqProbeWithLevels is an ffprobe answer for a PQ source whose stream carries
// HDR10 light levels.
//
// Parameters:
//   - sideData: The stream's side_data_list JSON.
//
// Returns:
//   - payload: The ffprobe JSON.
func pqProbeWithLevels(sideData string) string {
	return `{"format":{"duration":"60.0","bit_rate":"8000","format_name":"matroska"},` +
		`"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","width":3840,` +
		`"height":2160,"color_transfer":"smpte2084","side_data_list":` + sideData + `}]}`
}

// TestTonePeakReadsTheSourcesHDR10Metadata covers the PQ peak: MaxCLL wins,
// the mastering display's peak follows, and the luma sample is the fallback
// for a source that carries neither, so clips of one title share exposure.
func TestTonePeakReadsTheSourcesHDR10Metadata(t *testing.T) {
	t.Parallel()

	sampled := tonemap.PeakFromNits(tonemap.NitsFromLimitedY(158))

	tests := []struct {
		name     string
		sideData string
		want     float64
	}{
		{
			name: "MaxCLL",
			sideData: `[{"side_data_type":"Content light level metadata","max_content":1000,"max_average":400},` +
				`{"side_data_type":"Mastering display metadata","max_luminance":"4000/1"}]`,
			want: 10,
		},
		{
			name: "mastering peak when MaxCLL is unknown",
			sideData: `[{"side_data_type":"Content light level metadata","max_content":0,"max_average":0},` +
				`{"side_data_type":"Mastering display metadata","max_luminance":"10000000/10000"}]`,
			want: 10,
		},
		{name: "sampled without metadata", sideData: `[]`, want: sampled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newEncodeFixture(t, "peak.mp4")
			runner := NewExecFFmpeg(
				passStub(t, signalstatsLog),
				probeStub(t, pqProbeWithLevels(test.sideData)),
			)

			peak := runner.tonePeak(
				t.Context(),
				fixture.input,
				clip.TransferPQAlias,
				0,
				time.Second,
			)

			assert.InDelta(t, test.want, peak, 0.0001)
		})
	}
}
