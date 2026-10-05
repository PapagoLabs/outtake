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
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/tonemap"
)

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

func TestRemapDecisionToneMapsOnlyForWebSafeColor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, preserveHDR, remapDecisionFor(hdrInputs{webSafe: false, preserve: false}),
		"neither flag still keeps the source HDR transfer")
	assert.Equal(t, preserveHDR, remapDecisionFor(hdrInputs{webSafe: false, preserve: true}))
	assert.Equal(t, webSafeRemap, remapDecisionFor(hdrInputs{webSafe: true, preserve: false}))
	assert.Equal(t, webSafeRemap, remapDecisionFor(hdrInputs{webSafe: true, preserve: true}),
		"an explicit web-safe request tone-maps even when preserve is also set")
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
			remap:       webSafeRemap,
			wantHDRKind: "",
			wantTags:    nameBT709,
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:        "an sdr source with an unreported transfer is left untagged",
			transfer:    "unknown",
			remap:       webSafeRemap,
			wantHDRKind: "",
			wantTags:    "",
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:        "a pal sdr source is not claimed as srgb",
			transfer:    "bt470bg",
			remap:       webSafeRemap,
			wantHDRKind: "",
			wantTags:    "bt470bg",
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:          "pq is remapped by default and tagged rec709",
			transfer:      clip.TransferPQ,
			remap:         webSafeRemap,
			wantHDRKind:   clip.TransferPQAlias,
			wantToneMap:   true,
			wantTags:      tonemap.TransferSRGB,
			wantNeedsPeak: true,
			wantPixFmt:    pixelFormatYUV420P,
		},
		{
			name:        "pq is preserved, tagged, and kept 10-bit",
			transfer:    clip.TransferPQ,
			remap:       preserveHDR,
			wantHDRKind: clip.TransferPQAlias,
			wantTags:    clip.TransferPQ,
			wantPixFmt:  pixelFormatYUV420P10LE,
		},
		{
			name:        "hlg is remapped, tagged rec709, and needs no peak sample",
			transfer:    clip.TransferHLG,
			remap:       webSafeRemap,
			wantHDRKind: clip.TransferHLGAlias,
			wantToneMap: true,
			wantTags:    tonemap.TransferSRGB,
			wantPixFmt:  pixelFormatYUV420P,
		},
		{
			name:        "hlg is preserved, tagged, and kept 10-bit",
			transfer:    clip.TransferHLG,
			remap:       preserveHDR,
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
	applyColorPlan(&req, clip.TransferPQ, preserveHDR)

	args := h264EncodeArgs(&req)
	joined := strings.Join(args, " ")

	assert.Contains(t, args, primariesBT2020)
	assert.Contains(t, args, clip.TransferPQ)
	assert.NotContains(t, joined, "tonemap=tonemap=hable",
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
			remap:    preserveHDR,
			want:     pixelFormatYUV420P10LE,
		},
		{
			name:     "preserved hlg is 10-bit",
			transfer: clip.TransferHLG,
			remap:    preserveHDR,
			want:     pixelFormatYUV420P10LE,
		},
		{
			name:     "remapped pq is 8-bit",
			transfer: clip.TransferPQ,
			remap:    webSafeRemap,
			want:     pixelFormatYUV420P,
		},
		{
			name:     "remapped hlg is 8-bit",
			transfer: clip.TransferHLG,
			remap:    webSafeRemap,
			want:     pixelFormatYUV420P,
		},
		{
			name:     "sdr is 8-bit",
			transfer: nameBT709,
			remap:    preserveHDR,
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
	execFFmpeg := NewExecFFmpeg(writePassStub(t, signalstatsLog), "unused")

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
	execFFmpeg := NewExecFFmpeg(stubScript(t, failingStubScript(signalstatsLog)), "unused")

	peak, ok := execFFmpeg.signalstatsYMax(t.Context(), path, 10*time.Second, 30*time.Second)
	require.True(t, ok, "the stubbed luma is still parsed and returned")
	assert.InDelta(t, 158.0, peak, 0.001)

	assert.False(
		t,
		peakCached(identity),
		"a pass that errored must not be cached",
	)
}

func TestSignalstatsDoesNotCacheFailure(t *testing.T) {
	t.Parallel()

	path, identity := newPassSource(t)
	execFFmpeg := NewExecFFmpeg(
		writePassStub(t, "no signalstats output here"),
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

	execFFmpeg := NewExecFFmpeg(writeSwappingPassStub(t, signalstatsLog), "unused")

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

	assert.True(t, strings.HasPrefix(signalstatsFilter, "format=yuv420p,"),
		"signalstats on a 10-bit source reports YMAX in 10-bit codes, and the peak math reads 8-bit")
	assert.Contains(t, signalstatsFilter, "signalstats")
}

func TestPeakSampleSecondsPinsTheWindow(t *testing.T) {
	t.Parallel()

	assert.Equal(t, webSafePeak, peakSampleSeconds(600*time.Second))
	assert.Equal(t, webSafePeak, peakSampleSeconds(0))
	assert.Equal(t, 3500*time.Millisecond, peakSampleSeconds(3500*time.Millisecond))
}
