// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package tonemap

import (
	"math"
	"regexp"
	"strconv"

	"github.com/PapagoLabs/outtake/internal/clip"
)

const (
	// DefaultWebSafePeak is 400 nits relative to a 100-nit SDR white.
	DefaultWebSafePeak = 4.0
	// TransferSRGB is the sRGB transfer function.
	TransferSRGB = "iec61966-2-1"
	// webSafeNPL is the zscale nominal peak luminance for SDR white in nits.
	webSafeNPL = 100.0
	// limitedRangeOffset is the TV-range luma black (code 16).
	limitedRangeOffset = 16.0
	// limitedRangeSpan is the TV-range luma extent (235 minus 16).
	limitedRangeSpan = 219.0
	// pqMaxNits is the SMPTE ST 2084 peak luminance in nits.
	pqMaxNits = 10000.0
	// pqC1 is the SMPTE ST 2084 c1 coefficient (3424/4096).
	pqC1 = 3424.0 / 4096.0
	// pqC2 is the SMPTE ST 2084 c2 coefficient (2413/128).
	pqC2 = 2413.0 / 128.0
	// pqC3 is the SMPTE ST 2084 c3 coefficient (2392/128).
	pqC3 = 2392.0 / 128.0
	// pqM is the SMPTE ST 2084 m exponent (2523/32).
	pqM = 2523.0 / 32.0
	// pqN is the SMPTE ST 2084 n exponent (2610/16384).
	pqN = 2610.0 / 16384.0
	// peakFormatPrec is the number of decimals on tonemap peak=.
	peakFormatPrec = 4
	// floatBitSize is the width ffmpeg renders these numbers at.
	floatBitSize = 64
)

// signalstatsYMaxPattern matches lavfi.signalstats YMAX lines.
var signalstatsYMaxPattern = regexp.MustCompile(`YMAX=([0-9.]+)`)

// NitsFromLimitedY converts a limited-range 8-bit luma code to PQ nits.
//
// Parameters:
//   - ymax: Limited-range luma code (16–235 typical).
//
// Returns:
//   - nits: Absolute luminance in nits.
func NitsFromLimitedY(ymax float64) float64 {
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

// PeakFromNits maps nits onto zscale npl=100 linear peak.
//
// Parameters:
//   - nits: Measured PQ peak luminance.
//
// Returns:
//   - peak: Relative peak for ffmpeg tonemap=peak, at least 1.
func PeakFromNits(nits float64) float64 {
	peak := nits / webSafeNPL
	if peak < 1 {
		return 1
	}

	return peak
}

// ParseSignalstatsYMax returns the highest YMAX in ffmpeg signalstats logs.
//
// Parameters:
//   - log: ffmpeg stderr text from a signalstats pass.
//
// Returns:
//   - ymax: The highest parsed YMAX value.
//   - ok: True when at least one YMAX was found.
func ParseSignalstatsYMax(log string) (float64, bool) {
	matches := signalstatsYMaxPattern.FindAllStringSubmatch(log, -1)
	if len(matches) == 0 {
		return 0, false
	}

	maxY := float64(0)
	found := false

	for _, match := range matches {
		value, err := strconv.ParseFloat(match[1], floatBitSize)
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

// ToneMapFilter is a CPU HDR to SDR filter chain.
//
// Parameters:
//   - hdrKind: clip.TransferPQAlias or clip.TransferHLGAlias.
//   - peak: Relative peak for tonemap=peak (npl=100). Values below 1 use
//     DefaultWebSafePeak.
//
// Returns:
//   - filter: An ffmpeg -vf fragment ending in sidedata=mode=delete.
func ToneMapFilter(hdrKind string, peak float64) string {
	if peak < 1 {
		peak = DefaultWebSafePeak
	}

	tin := clip.TransferPQ
	if hdrKind == clip.TransferHLGAlias {
		tin = clip.TransferHLG
	}

	return "zscale=tin=" + tin +
		":min=bt2020nc:pin=bt2020:rin=tv:t=linear:npl=100,format=gbrpf32le,zscale=p=bt709," +
		"tonemap=tonemap=hable:desat=0:peak=" +
		strconv.FormatFloat(peak, 'f', peakFormatPrec, floatBitSize) +
		",zscale=t=iec61966-2-1:m=bt709:p=bt709:r=tv,format=yuv420p,sidedata=mode=delete"
}
