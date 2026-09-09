// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package crop

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

// Size is a decoded video frame size from ffmpeg logs.
type Size struct {
	Width  int
	Height int
}

const (
	// cropdetectLimit is the 8-bit black threshold used as limit=N/255.
	cropdetectLimit = 24
	// cropdetectLimitDen is the 8-bit range for the cropdetect limit.
	cropdetectLimitDen = 255
	// cropdetectRound rounds crop dimensions to even values.
	cropdetectRound = 2
	// cropdetectReset keeps running bounds across sampled frames.
	cropdetectReset = 0
	// cropdetectMaxSecs is how long a cropdetect pass may sample.
	cropdetectMaxSecs = 3
	// nullOutput is ffmpeg's null muxer sink.
	nullOutput = "-"
	// outputFlag is the FFmpeg overwrite flag.
	outputFlag = "-y"
	// ssFlag is the FFmpeg seek flag.
	ssFlag = "-ss"
	// inputFlag is the FFmpeg input flag.
	inputFlag = "-i"
	// durationFlag is the FFmpeg duration flag.
	durationFlag = "-t"
	// videoFilterFlag is the FFmpeg video-filter flag.
	videoFilterFlag = "-vf"
)

// cropdetectPattern matches the last crop=W:H:X:Y line from ffmpeg cropdetect.
var cropdetectPattern = regexp.MustCompile(`crop=(\d+):(\d+):(\d+):(\d+)`)

// streamSizePattern matches the decoded video WxH in ffmpeg logs.
var streamSizePattern = regexp.MustCompile(`Video:.*?(\d+)x(\d+)`)

// Filter returns the ffmpeg crop filter argument.
//
// Returns:
//   - filter: The crop=W:H:X:Y filter string.
func (rect CropRect) Filter() string {
	return fmt.Sprintf("crop=%d:%d:%d:%d", rect.Width, rect.Height, rect.X, rect.Y)
}

// Trims reports whether crop removes bars from a source frame.
//
// Parameters:
//   - srcWidth: Source frame width in pixels.
//   - srcHeight: Source frame height in pixels.
//
// Returns:
//   - ok: True when the rectangle is valid and removes enough pixels.
func (rect CropRect) Trims(srcWidth, srcHeight int) bool {
	if !rect.Valid() {
		return false
	}

	if srcWidth <= 0 || srcHeight <= 0 {
		return true
	}

	removed := (srcWidth - rect.Width) + (srcHeight - rect.Height)

	return removed >= cropdetectRound*2
}

// TrimsLog reports whether rect removes bars from the frame in log.
//
// Parameters:
//   - output: ffmpeg stderr text that may include a Video: WxH line.
//
// Returns:
//   - ok: True when the rectangle is valid and removes enough pixels.
func (rect CropRect) TrimsLog(output string) bool {
	size := ParseStreamSize(output)

	return rect.Trims(size.Width, size.Height)
}

// Valid reports whether the rectangle can be applied as a crop.
//
// Returns:
//   - ok: True when width and height are positive.
func (rect CropRect) Valid() bool {
	return rect.Width > 0 && rect.Height > 0
}

// ParseCropdetect returns the last crop=W:H:X:Y from cropdetect logs.
//
// Parameters:
//   - output: ffmpeg stderr text from a cropdetect pass.
//
// Returns:
//   - crop: The last parsed rectangle, or a zero value.
//   - ok: True when a valid rectangle was found.
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

	rect := CropRect{Width: width, Height: height, X: offsetX, Y: offsetY}
	if !rect.Valid() {
		return CropRect{}, false
	}

	return rect, true
}

// DetectArgs builds the ffmpeg argv for a cropdetect pass.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - start: Seek offset in seconds.
//   - duration: Clip duration in seconds; capped at three seconds.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func DetectArgs(ffmpegPath, input string, start, duration float64) []string {
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

// cropdetectDuration caps how long cropdetect samples the source.
//
// Parameters:
//   - duration: Duration.
//
// Returns:
//   - value: The value.
func cropdetectDuration(duration float64) float64 {
	if duration <= 0 {
		return cropdetectMaxSecs
	}

	return min(duration, cropdetectMaxSecs)
}

// ParseStreamSize reads the source video width and height from ffmpeg logs.
//
// Parameters:
//   - output: ffmpeg stderr text.
//
// Returns:
//   - size: Frame size, or a zero value when missing.
func ParseStreamSize(output string) Size {
	match := streamSizePattern.FindStringSubmatch(output)
	if match == nil {
		return Size{}
	}

	parsedWidth, widthErr := strconv.Atoi(match[1])
	parsedHeight, heightErr := strconv.Atoi(match[2])
	if widthErr != nil || heightErr != nil {
		return Size{}
	}

	return Size{Width: parsedWidth, Height: parsedHeight}
}

// formatDuration formats a duration in seconds to a string.
//
// Parameters:
//   - seconds: Seconds.
//
// Returns:
//   - value: The value.
func formatDuration(seconds float64) string {
	return fmt.Sprintf("%.3f", seconds)
}
