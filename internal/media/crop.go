// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"fmt"
	"regexp"
	"strconv"
)

// CropRect is an ffmpeg crop=W:H:X:Y rectangle.
type CropRect struct {
	Width  int
	Height int
	X      int
	Y      int
}

// streamSize is a decoded video frame size from ffmpeg logs.
type streamSize struct {
	width  int
	height int
}

const (
	// CropdetectLimit is the 8-bit black threshold used as limit=N/255.
	cropdetectLimit = 24
	// CropdetectLimitDen is the 8-bit range for the cropdetect limit.
	cropdetectLimitDen = 255
	// CropdetectRound rounds crop dimensions to even values.
	cropdetectRound = 2
	// CropdetectReset keeps running bounds across sampled frames.
	cropdetectReset = 0
	// CropdetectMaxSecs is how long a cropdetect pass may sample.
	cropdetectMaxSecs = 3
	// NullOutput is ffmpeg's null muxer sink.
	nullOutput = "-"
)

// cropdetectPattern matches the last crop=W:H:X:Y line from ffmpeg cropdetect.
var cropdetectPattern = regexp.MustCompile(`crop=(\d+):(\d+):(\d+):(\d+)`)

// streamSizePattern matches the decoded video WxH in ffmpeg logs.
var streamSizePattern = regexp.MustCompile(`Video:.*?(\d+)x(\d+)`)

// Filter returns the ffmpeg crop filter argument.
func (crop CropRect) Filter() string {
	return fmt.Sprintf("crop=%d:%d:%d:%d", crop.Width, crop.Height, crop.X, crop.Y)
}

// Trims reports whether crop removes bars from a source frame.
func (crop CropRect) Trims(srcWidth, srcHeight int) bool {
	if !crop.Valid() {
		return false
	}

	if srcWidth <= 0 || srcHeight <= 0 {
		return true
	}

	removed := (srcWidth - crop.Width) + (srcHeight - crop.Height)

	return removed >= cropdetectRound*2
}

// Valid reports whether the rectangle can be applied as a crop.
func (crop CropRect) Valid() bool {
	return crop.Width > 0 && crop.Height > 0
}

// ParseCropdetect returns the last crop=W:H:X:Y from cropdetect logs.
func ParseCropdetect(output string) (CropRect, bool) {
	matches := cropdetectPattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return CropRect{}, false
	}

	last := matches[len(matches)-1]
	width, err := strconv.Atoi(last[1])
	if err != nil {
		return CropRect{}, false
	}

	height, err := strconv.Atoi(last[2])
	if err != nil {
		return CropRect{}, false
	}

	offsetX, err := strconv.Atoi(last[3])
	if err != nil {
		return CropRect{}, false
	}

	offsetY, err := strconv.Atoi(last[4])
	if err != nil {
		return CropRect{}, false
	}

	crop := CropRect{Width: width, Height: height, X: offsetX, Y: offsetY}
	if !crop.Valid() {
		return CropRect{}, false
	}

	return crop, true
}

// cropdetectDuration caps how long cropdetect samples the source.
func cropdetectDuration(duration float64) float64 {
	if duration <= 0 {
		return cropdetectMaxSecs
	}

	return min(duration, cropdetectMaxSecs)
}

// parseStreamSize reads the source video width and height from ffmpeg logs.
func parseStreamSize(output string) streamSize {
	match := streamSizePattern.FindStringSubmatch(output)
	if match == nil {
		return streamSize{}
	}

	parsedWidth, widthErr := strconv.Atoi(match[1])
	parsedHeight, heightErr := strconv.Atoi(match[2])
	if widthErr != nil || heightErr != nil {
		return streamSize{}
	}

	return streamSize{width: parsedWidth, height: parsedHeight}
}

// cropdetectArgs builds the ffmpeg argv for a cropdetect pass.
func cropdetectArgs(ffmpegPath, input string, start, duration float64) []string {
	return []string{
		ffmpegPath,
		outputFlag,
		ssFlag, formatDuration(start),
		inputFlag, input,
		durationFlag, formatDuration(cropdetectDuration(duration)),
		videoFilterFlag,
		fmt.Sprintf(
			"format=yuv420p,cropdetect=limit=%d/%d:round=%d:reset=%d",
			cropdetectLimit,
			cropdetectLimitDen,
			cropdetectRound,
			cropdetectReset,
		),
		"-an",
		"-f", "null",
		nullOutput,
	}
}
