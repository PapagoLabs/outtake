// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"context"
	"strings"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/tonemap"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// hdrInputs are the settings that decide how an HDR source is handled.
type hdrInputs struct {
	// webSafe reports that web-safe color is enabled for the request.
	webSafe bool
	// preserve reports that the preset asked to keep the source's HDR transfer.
	preserve bool
}

// remapDecision is how an encode treats an HDR source.
type remapDecision int

// colorPlan is how an encode handles a source's color.
type colorPlan struct {
	// hdrKind is the transfer alias the tone map filter expects, empty for SDR.
	hdrKind string
	// toneMap reports that the tone map chain should run.
	toneMap bool
	// colorTags describe the output's color.
	colorTags []string
	// needsPeak reports that the PQ peak still has to be sampled.
	needsPeak bool
	// pixFmt is the pixel format the output should carry.
	pixFmt string
}

const (
	// preserveHDR keeps a source's HDR transfer as it is.
	preserveHDR remapDecision = iota
	// webSafeRemap tone maps an HDR source down to Rec.709.
	webSafeRemap
)

const (
	// webSafeMovFlags adds a colr atom so browsers agree on Rec.709.
	webSafeMovFlags = "+faststart+write_colr"
	// webSafePeak caps how long a luma-peak pass may sample.
	webSafePeak = 8 * time.Second
	// signalstatsFilter prints lavfi.signalstats.YMAX to stderr for peak detect.
	// The 8-bit conversion comes first: NitsFromLimitedY reads an 8-bit limited
	// code, and a 10-bit YMAX above 235 would clamp to the full 10000-nit PQ peak.
	signalstatsFilter = "format=yuv420p,signalstats,metadata=mode=print"
	// signalstatsPass names the luma sampling pass in logs.
	signalstatsPass = "signalstats"
)

const (
	// nameBT709 is the name ffmpeg uses for the BT.709 primaries, matrix, and transfer alike.
	nameBT709 = "bt709"
	// primariesBT2020 is the BT.2020 primaries name ffmpeg expects.
	primariesBT2020 = "bt2020"
	// matrixBT2020NC is the BT.2020 non-constant luminance matrix.
	matrixBT2020NC = "bt2020nc"
	// transferSDRUnknown is what ffprobe reports for a stream with no transfer.
	transferSDRUnknown = "unknown"
	// pixelFormatYUV420P is the browser-safe 8-bit 4:2:0 pixel format.
	pixelFormatYUV420P = "yuv420p"
	// pixelFormatYUV420P10LE is the 10-bit 4:2:0 pixel format.
	pixelFormatYUV420P10LE = "yuv420p10le"
	// pixelFormatFlag is the FFmpeg pixel-format flag.
	pixelFormatFlag = "-pix_fmt"
	// flagColorPrimaries is the ffmpeg primaries flag, shared by every tagging path.
	flagColorPrimaries = "-color_primaries"
	// flagColorTransfer is the ffmpeg transfer flag.
	flagColorTransfer = "-color_trc"
	// flagColorSpace is the ffmpeg matrix flag.
	flagColorSpace = "-colorspace"
	// flagColorRange is the ffmpeg range flag.
	flagColorRange = "-color_range"
	// flagX264Params carries the same values in x264's own spelling.
	flagX264Params = "-x264-params"
	// flagRangeTV is the limited range every tag here describes.
	flagRangeTV = "tv"

	// x264PrimariesPrefix spells the primaries field with its own prefix.
	x264PrimariesPrefix = "colorprim="
	// x264TransferPrefix is the x264-params transfer prefix.
	x264TransferPrefix = ":transfer="
	// x264MatrixPrefix is the x264-params matrix prefix.
	x264MatrixPrefix = ":colormatrix="
)

// remapDecisionFor maps an encode request onto the HDR handling it needs.
//
// Parameters:
//   - in: The request's HDR-relevant settings.
//
// Returns:
//   - decision: Whether the encode tone maps an HDR source.
func remapDecisionFor(in hdrInputs) remapDecision {
	// Web-safe color is the only request that tone-maps. Leaving it off keeps
	// the source transfer, including when preserve is also off, so a file the
	// user will grade is not graded here. An explicit web-safe request still
	// wins when both flags are set.
	if in.webSafe {
		return webSafeRemap
	}

	return preserveHDR
}

// hdrColorArgs tags an encode with the HDR transfer it actually carries.
//
// Parameters:
//   - kind: Transfer alias, clip.TransferPQAlias or clip.TransferHLGAlias.
//
// Returns:
//   - args: ffmpeg color and x264-params flags.
func hdrColorArgs(kind string) []string {
	transfer := clip.TransferPQ
	if kind == clip.TransferHLGAlias {
		transfer = clip.TransferHLG
	}

	return []string{
		flagColorPrimaries, primariesBT2020,
		flagColorTransfer, transfer,
		flagColorSpace, matrixBT2020NC,
		flagColorRange, "tv",
		flagX264Params, "colorprim=" + primariesBT2020 + ":transfer=" + transfer +
			":colormatrix=" + matrixBT2020NC,
	}
}

// webSafeColorArgs tags the encode as Rec.709 / sRGB limited range.
//
// Returns:
//   - args: ffmpeg color and x264-params flags.
func webSafeColorArgs() []string {
	return []string{
		flagColorPrimaries, nameBT709,
		flagColorTransfer, tonemap.TransferSRGB,
		flagColorSpace, nameBT709,
		flagColorRange, flagRangeTV,
		flagX264Params, "colorprim=" + nameBT709 + ":transfer=" + tonemap.TransferSRGB +
			":colormatrix=" + nameBT709,
	}
}

// sdrColorArgs tags a passthrough of an SDR source with the transfer it carries.
//
// Parameters:
//   - transfer: ffprobe's color_transfer for the source.
//
// Returns:
//   - args: ffmpeg color and x264-params flags, empty when the transfer is unknown.
func sdrColorArgs(transfer string) []string {
	name := strings.ToLower(strings.TrimSpace(transfer))
	if name == "" || name == transferSDRUnknown {
		return nil
	}

	return []string{
		flagColorPrimaries, nameBT709,
		flagColorTransfer, name,
		flagColorSpace, nameBT709,
		flagColorRange, flagRangeTV,
		flagX264Params, x264PrimariesPrefix + nameBT709 +
			x264TransferPrefix + name +
			x264MatrixPrefix + nameBT709,
	}
}

// decideColor maps a source's transfer onto an encode's color handling.
//
// Parameters:
//   - transfer: ffprobe's color transfer, empty for SDR.
//   - remap: Why an HDR source is being handled at all.
//
// Returns:
//   - plan: What the encode should do.
func decideColor(transfer string, remap remapDecision) colorPlan {
	if !clip.IsHDRTransfer(transfer) {
		return colorPlan{colorTags: sdrColorArgs(transfer), pixFmt: pixelFormatYUV420P}
	}

	kind := clip.TransferPQAlias
	if clip.IsHLGTransfer(transfer) {
		kind = clip.TransferHLGAlias
	}

	// A remapped encode outputs Rec.709 whatever it was given, so it is tagged
	// Rec.709. A PQ peak still has to be sampled to scale the tone map; HLG
	// carries a nominal peak and needs no sample.
	if remap == webSafeRemap {
		return colorPlan{
			hdrKind:   kind,
			toneMap:   true,
			colorTags: webSafeColorArgs(),
			needsPeak: kind == clip.TransferPQAlias,
			pixFmt:    pixelFormatYUV420P,
		}
	}

	return colorPlan{
		hdrKind:   kind,
		colorTags: hdrColorArgs(kind),
		pixFmt:    pixelFormatYUV420P10LE,
	}
}

// resolveColor decides how an encode handles the source's color.
//
// Parameters:
//   - ctx: Cancellation and deadline for probe and luma sampling.
//   - req: Encode request to update in place.
func (execFFmpeg *ExecFFmpeg) resolveColor(ctx context.Context, req *h264EncodeRequest) {
	req.hdrKind = ""
	req.toneMap = false
	req.colorTags = nil
	req.tonePeak = 0

	info, err := execFFmpeg.Probe(ctx, req.input)
	if err != nil {
		// Untagged, as before. That beats tagging a source we could not identify.
		return
	}

	transfer := ""
	if clip.IsHDRTransfer(info.ColorTransfer) {
		transfer = info.ColorTransfer
	}

	plan := decideColor(transfer, remapDecisionFor(hdrInputs{
		webSafe:  req.webSafeColor,
		preserve: req.preset.PreserveHDR,
	}))

	req.hdrKind = plan.hdrKind
	req.toneMap = plan.toneMap
	req.colorTags = plan.colorTags
	req.pixFmt = plan.pixFmt

	if plan.hdrKind == clip.TransferHLGAlias {
		req.tonePeak = tonemap.DefaultWebSafePeak
	}

	if !plan.needsPeak {
		return
	}

	ymax, ok := execFFmpeg.signalstatsYMax(ctx, req.input, req.start, req.duration)
	if !ok {
		// Tone map against the nominal peak rather than abandoning the map. The
		// tags already say Rec.709, so leaving the source's HDR pixels in place
		// would ship them mislabelled.
		req.tonePeak = tonemap.DefaultWebSafePeak

		return
	}

	req.tonePeak = tonemap.PeakFromNits(tonemap.NitsFromLimitedY(ymax))
}

// signalstatsYMax samples luma on a short window of the clip.
//
// Parameters:
//   - ctx: Cancellation and deadline for the ffmpeg pass.
//   - input: Source media path.
//   - start: Seek offset into the source.
//   - duration: Length of the clip; capped at webSafePeak.
//
// Returns:
//   - ymax: Highest limited-range luma code observed.
//   - ok: True when at least one YMAX value was parsed.
func (execFFmpeg *ExecFFmpeg) signalstatsYMax(
	ctx context.Context,
	input string,
	start, duration time.Duration,
) (float64, bool) {
	cleanInput, err := mediaPath(input)
	if err != nil {
		return 0, false
	}

	window := peakSampleSeconds(duration)
	key, _ := probe.KeyFor(cleanInput)

	if cached, found := key.CachedPeak(start, window); found {
		return cached, true
	}

	args := []string{
		execFFmpeg.ffmpegPath,
		ssFlag, timecode.FromDuration(start).FormatSeconds(),
		inputFlag, cleanInput,
		durationFlag, timecode.FromDuration(window).FormatSeconds(),
		anFlag,
		videoFilterFlag, signalstatsFilter,
		"-f", "null",
		"-",
	}

	stderr, runErr := execFFmpeg.runStderr(ctx, signalstatsPass, args...)

	ymax, ok := tonemap.ParseSignalstatsYMax(stderr)

	// Only a clean pass is cached. A pass that produced no luma is not necessarily
	// a source with no highlights, and a pass that errored — a timeout most of
	// all — can have emitted a YMAX from a partial sample, which would understate
	// the peak and mis-scale the tone map.
	if ok && runErr == nil {
		probe.StorePeak(cleanInput, key, start, window, ymax)
	}

	return ymax, ok
}

// peakSampleSeconds returns how long signalstats samples the source.
//
// Parameters:
//   - duration: Requested clip duration.
//
// Returns:
//   - capped: The sampling window, capped at webSafePeak.
func peakSampleSeconds(duration time.Duration) time.Duration {
	if duration <= 0 || duration > webSafePeak {
		return webSafePeak
	}

	return duration
}
