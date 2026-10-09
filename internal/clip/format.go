// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"strconv"
)

// Format is what a rendered video file holds, read from the file when it is
// published, so a player can say what it plays.
type Format struct {
	// Width is the frame width in pixels, zero when it was never read.
	Width int
	// Height is the frame height in pixels.
	Height int
	// HDR reports that the file carries an HDR transfer.
	HDR bool
}

// namedSize is a size a file is named after, at 16:9.
type namedSize struct {
	// width is the export width, which names the size.
	width int
	// height is the frame height at 16:9.
	height int
}

const (
	// rangeHDR names a file that carries HDR.
	rangeHDR = "HDR"
	// rangeSDR names a file that does not.
	rangeSDR = "SDR"
	// labelSeparator joins the range and the resolution.
	labelSeparator = " · "
	// sizeSeparator joins a width and a height that have no name.
	sizeSeparator = "×"
	// sizeTolerancePercent is how far a file's width or height may differ
	// from a named size and still take its name, which covers the few pixels
	// a black-bar trim removes.
	sizeTolerancePercent = 2
	// percent is the denominator of sizeTolerancePercent.
	percent = 100
	// height720p is 720p's height at 16:9.
	height720p = 720
	// height1080p is 1080p's height at 16:9.
	height1080p = 1080
	// height1440p is 1440p's height at 16:9.
	height1440p = 1440
	// height2160p is 4K's height at 16:9.
	height2160p = 2160
)

// namedSizes lists the sizes a file is named after, largest first, so a file
// as wide as one and as tall as a smaller one takes the larger name.
var namedSizes = []namedSize{
	{width: OutputWidth2160p, height: height2160p},
	{width: OutputWidth1440p, height: height1440p},
	{width: OutputWidth1080p, height: height1080p},
	{width: OutputWidth720p, height: height720p},
}

// Known reports whether the file's format was read.
//
// Returns:
//   - known: True once the file was probed after it was published.
func (format Format) Known() bool {
	return format.Width > 0 && format.Height > 0
}

// Label names the file's range and resolution, such as "HDR · 4K". A size
// within sizeTolerancePercent of an export resolution's width or height is
// named as the profile editor names it, so a 4K clip whose black bars were
// trimmed is still 4K and a 4:3 clip 1080 lines tall is 1080p. Any other size
// is written out, such as "720×480".
//
// Returns:
//   - label: The badge text, empty when the format was never read.
func (format Format) Label() string {
	if !format.Known() {
		return ""
	}

	dynamicRange := rangeSDR
	if format.HDR {
		dynamicRange = rangeHDR
	}

	return dynamicRange + labelSeparator + format.resolution()
}

// resolution names the file's size.
//
// Returns:
//   - name: An export resolution's name, or the width and height.
func (format Format) resolution() string {
	for _, size := range namedSizes {
		if nearSize(format.Width, size.width) || nearSize(format.Height, size.height) {
			return OutputWidthLabel(size.width)
		}
	}

	return strconv.Itoa(format.Width) + sizeSeparator + strconv.Itoa(format.Height)
}

// nearSize reports whether a measured dimension is within
// sizeTolerancePercent of a named one.
//
// Parameters:
//   - measured: The file's width or height.
//   - named: The named size's width or height.
//
// Returns:
//   - near: True when the two differ by no more than the tolerance.
func nearSize(measured, named int) bool {
	difference := max(measured-named, named-measured)

	return difference*percent <= named*sizeTolerancePercent
}
