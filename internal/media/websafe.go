// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"math"
	"regexp"
	"strconv"
	"strings"
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

const (
	// PreserveHDRDecision keeps a source's HDR transfer as it is.
	preserveHDR remapDecision = iota
	// WebSafeRemapDecision tone maps an HDR source down to Rec.709.
	webSafeRemap
)

const (
	// DefaultWebSafePeak is 400 nits relative to a 100-nit SDR white.
	// Used when luma cannot be measured. Do not use disc MaxCLL tags.
	defaultWebSafePeak = 4.0
	// WebSafeNPL is zscale nominal peak luminance for SDR white in nits.
	webSafeNPL = 100.0
	// LimitedRangeOffset is TV-range luma black (code 16).
	limitedRangeOffset = 16.0
	// LimitedRangeSpan is TV-range luma extent (235 minus 16).
	limitedRangeSpan = 219.0
	// PQMaxNits is SMPTE ST 2084 peak luminance in nits.
	pqMaxNits = 10000.0
	// PqC1 is the SMPTE ST 2084 c1 coefficient (3424/4096).
	pqC1 = 3424.0 / 4096.0
	// PqC2 is the SMPTE ST 2084 c2 coefficient (2413/128).
	pqC2 = 2413.0 / 128.0
	// PqC3 is the SMPTE ST 2084 c3 coefficient (2392/128).
	pqC3 = 2392.0 / 128.0
	// PqM is the SMPTE ST 2084 m exponent (2523/32).
	pqM = 2523.0 / 32.0
	// PqN is the SMPTE ST 2084 n exponent (2610/16384).
	pqN = 2610.0 / 16384.0
	// TransferPQ is ffprobe's HDR10 / PQ transfer name.
	transferPQ = "smpte2084"
	// TransferHLG is ffprobe's HLG transfer name.
	transferHLG = "arib-std-b67"
	// TransferPQAlias is a short name some probes use for PQ.
	transferPQAlias = "pq"
	// TransferHLGAlias is a short name some probes use for HLG.
	transferHLGAlias = "hlg"
	// PrimariesBT2020 is the BT.2020 primaries name ffmpeg expects.
	primariesBT2020 = "bt2020"
	// MatrixBT2020NC is the BT.2020 non-constant luminance matrix.
	matrixBT2020NC = "bt2020nc"
	// TransferRec709 is the sRGB transfer ffmpeg expects for Rec.709 output.
	TransferRec709 = "iec61966-2-1"
	// TransferRec709Probe is ffprobe's Rec.709 transfer name, for SDR sources.
	transferRec709Probe = "bt709"
	// PeakFormatPrec is the number of decimals on tonemap peak=.
	peakFormatPrec = 4
)

// signalstatsYMaxPattern matches lavfi.signalstats YMAX lines.
var signalstatsYMaxPattern = regexp.MustCompile(`YMAX=([0-9.]+)`)

// remapDecisionFor maps an encode request onto the HDR handling it needs.
//
// An HDR source is tone mapped unless web-safe color is off and the preset asked
// to keep it.
//
// Parameters:
//   - in: The request's HDR-relevant settings.
//
// Returns:
//   - decision: Why an HDR source is being handled.
func remapDecisionFor(in hdrInputs) remapDecision {
	if in.webSafe || !in.preserve {
		return webSafeRemap
	}

	return preserveHDR
}

// hdrColorArgs tags an encode with the HDR transfer it actually carries.
//
// Used when a source is preserved rather than tone mapped. Leaving those files
// untagged is the defect this branch addresses, so a preserved source is
// described rather than left for the player to guess at.
//
// Parameters:
//   - kind: Transfer alias, transferPQAlias or transferHLGAlias.
//
// Returns:
//   - args: ffmpeg color and x264-params flags.
func hdrColorArgs(kind string) []string {
	transfer := transferPQ
	if kind == transferHLGAlias {
		transfer = transferHLG
	}

	return []string{
		"-color_primaries", primariesBT2020,
		"-color_trc", transfer,
		"-colorspace", matrixBT2020NC,
		"-color_range", "tv",
		"-x264-params", "colorprim=" + primariesBT2020 + ":transfer=" + transfer +
			":colormatrix=" + matrixBT2020NC,
	}
}

// isPQTransfer reports whether ffprobe color_transfer is HDR10/PQ.
//
// Parameters:
//   - transfer: ffprobe color_transfer value.
//
// Returns:
//   - ok: True when the transfer is PQ.
func isPQTransfer(transfer string) bool {
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case transferPQ, transferPQAlias:
		return true
	default:
		return false
	}
}

// isHLGTransfer reports whether ffprobe color_transfer is HLG.
//
// Parameters:
//   - transfer: ffprobe color_transfer value.
//
// Returns:
//   - ok: True when the transfer is HLG.
func isHLGTransfer(transfer string) bool {
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case transferHLG, transferHLGAlias:
		return true
	default:
		return false
	}
}

// isHDRTransfer reports whether the stream is HDR (PQ or HLG).
//
// Parameters:
//   - transfer: ffprobe color_transfer value.
//
// Returns:
//   - ok: True when the transfer is PQ or HLG.
func isHDRTransfer(transfer string) bool {
	return isPQTransfer(transfer) || isHLGTransfer(transfer)
}

// pqNitsFromLimitedY converts a limited-range 8-bit luma code to PQ nits.
//
// Parameters:
//   - ymax: Limited-range luma code (16–235 typical).
//
// Returns:
//   - nits: Absolute luminance in nits.
func pqNitsFromLimitedY(ymax float64) float64 {
	normalized := (ymax - limitedRangeOffset) / limitedRangeSpan
	if normalized <= 0 {
		return 0
	}

	if normalized > 1 {
		normalized = 1
	}

	raised := math.Pow(normalized, 1/pqM)
	den := pqC2 - pqC3*raised
	if den <= 0 {
		return 0
	}

	return pqMaxNits * math.Pow(math.Max(raised-pqC1, 0)/den, 1/pqN)
}

// tonePeakFromNits maps nits onto zscale npl=100 linear peak.
//
// Parameters:
//   - nits: Measured PQ peak luminance.
//
// Returns:
//   - peak: Relative peak for ffmpeg tonemap=peak, at least 1.
func tonePeakFromNits(nits float64) float64 {
	peak := nits / webSafeNPL
	if peak < 1 {
		return 1
	}

	return peak
}

// parseSignalstatsYMax returns the highest YMAX in ffmpeg signalstats logs.
//
// Parameters:
//   - log: ffmpeg stderr text from a signalstats pass.
//
// Returns:
//   - ymax: The highest parsed YMAX value.
//   - ok: True when at least one YMAX was found.
func parseSignalstatsYMax(log string) (float64, bool) {
	matches := signalstatsYMaxPattern.FindAllStringSubmatch(log, -1)
	if len(matches) == 0 {
		return 0, false
	}

	maxY := float64(0)
	found := false

	for _, match := range matches {
		value, err := strconv.ParseFloat(match[1], bitRateBits)
		if err != nil {
			continue
		}

		if !found || value > maxY {
			maxY = value
			found = true
		}
	}

	return maxY, found
}

// webSafeToneMapFilter is a CPU HDR to SDR filter chain.
//
// Parameters:
//   - hdrKind: transferPQAlias or transferHLGAlias.
//   - peak: Relative peak for tonemap=peak (npl=100). Values below 1 use
//     defaultWebSafePeak.
//
// Returns:
//   - filter: An ffmpeg -vf fragment ending in sidedata=mode=delete.
func webSafeToneMapFilter(hdrKind string, peak float64) string {
	if peak < 1 {
		peak = defaultWebSafePeak
	}

	tin := transferPQ
	if hdrKind == transferHLGAlias {
		tin = transferHLG
	}

	return "zscale=tin=" + tin +
		":min=bt2020nc:pin=bt2020:rin=tv:t=linear:npl=100,format=gbrpf32le,zscale=p=bt709," +
		"tonemap=tonemap=hable:desat=0:peak=" +
		strconv.FormatFloat(peak, 'f', peakFormatPrec, bitRateBits) +
		",zscale=t=iec61966-2-1:m=bt709:p=bt709:r=tv,format=yuv420p,sidedata=mode=delete"
}
