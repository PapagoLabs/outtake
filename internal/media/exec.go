// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/PapagoLabs/outtake/internal/logging"
	"github.com/PapagoLabs/outtake/internal/media/crop"
	"github.com/PapagoLabs/outtake/internal/media/progress"
)

// ExecFFmpeg provides FFmpeg execution capabilities.
type ExecFFmpeg struct {
	ffmpegPath  string
	ffprobePath string
	timeout     time.Duration
}

// h264EncodeRequest is the input for a browser-safe libx264 encode.
type h264EncodeRequest struct {
	ffmpegPath string
	input      string
	output     string
	start      float64
	duration   float64
	preset     QualityPreset
	audioIndex int
	maxWidth   int
	scaleFlags string
	crop       CropRect
}

const (
	// DefaultFPS is the default frames per second.
	defaultFPS = 10
	// DefaultWidth is the default width.
	defaultWidth = 480
	// ProbeTimeoutSec is the probe timeout in seconds.
	probeTimeoutSec = 30
	// OutputFlag is the FFmpeg output flag.
	outputFlag = "-y"
	// SsFlag is the FFmpeg seek flag.
	ssFlag = "-ss"
	// InputFlag is the FFmpeg input flag.
	inputFlag = "-i"
	// DurationFlag is the FFmpeg duration flag.
	durationFlag = "-t"
	// FramesFlag is the FFmpeg frames flag.
	framesFlag = "-frames:v"
	// QualityFlag is the FFmpeg quality flag.
	qualityFlag = "-q:v"
	// OverwriteFlag is the FFmpeg overwrite flag.
	overwriteFlag = "+faststart"
	// DefaultVideoCodec is the default video codec.
	defaultVideoCodec = "libx264"
	// DefaultAudioCodec is the default audio codec.
	defaultAudioCodec = "aac"
	// PixelFormatYUV420P is the browser-safe 8-bit 4:2:0 pixel format.
	pixelFormatYUV420P = "yuv420p"
	// PixelFormatFlag is the FFmpeg pixel-format flag.
	pixelFormatFlag = "-pix_fmt"
	// VideoFilterFlag is the FFmpeg video-filter flag.
	videoFilterFlag = "-vf"
	// ScaleFlagsLanczos is used for saved clip scaling.
	scaleFlagsLanczos = "lanczos"
	// ScaleFlagsFast is used for preview scaling.
	scaleFlagsFast = "fast_bilinear"
	// PreviewMaxWidth is the maximum width for preview encodes.
	previewMaxWidth = 1280
	// PreviewCRF is the libx264 CRF for previews.
	previewCRF = 30
	// PreviewPreset is the libx264 preset for previews.
	previewPreset = "ultrafast"
	// PreviewAudioKbps is the AAC bitrate for previews.
	previewAudioKbps = 96
	// PreviewMaxSecs caps how long a preview encode may run.
	previewMaxSecs = 30
)

// Type check.
var _ FFmpeg = (*ExecFFmpeg)(nil)

// NewExecFFmpeg creates a new FFmpeg executor.
func NewExecFFmpeg(ffmpegPath, ffprobePath string) *ExecFFmpeg {
	return &ExecFFmpeg{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		timeout:     DefaultFFmpegTimeout(),
	}
}

// DetectCrop samples the source with cropdetect and returns a crop rectangle.
func (execFFmpeg *ExecFFmpeg) DetectCrop(
	ctx context.Context,
	input string,
	start, duration float64,
) (CropRect, error) {
	cleanInput := filepath.Clean(input)
	args := crop.DetectArgs(execFFmpeg.ffmpegPath, cleanInput, start, duration)

	detectCtx, cancel := context.WithTimeout(ctx, execFFmpeg.timeout)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(detectCtx, args[0], args[1:]...)

	cmd.Dir = "/"

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logging.Logger.Debug().Err(err).Msg("cropdetect finished")
	}

	log := stderr.String()
	detected, ok := ParseCropdetect(log)
	if !ok || !detected.TrimsLog(log) {
		if err != nil && !ok {
			return CropRect{}, fmt.Errorf("cropdetect: %w", err)
		}

		return CropRect{}, nil
	}

	return detected, nil
}

// ExtractClip extracts a clip from a video.
func (execFFmpeg *ExecFFmpeg) ExtractClip(
	ctx context.Context,
	input, output string,
	start, duration float64,
	preset QualityPreset,
	audioIndex int,
	rect CropRect,
) error {
	// Build and run the clip ffmpeg command.
	cleanInput := filepath.Clean(input)
	cleanOutput := filepath.Clean(output)

	args := clipEncodeArgs(
		execFFmpeg.ffmpegPath,
		cleanInput,
		cleanOutput,
		start,
		duration,
		preset,
		audioIndex,
		rect,
	)

	err := execFFmpeg.run(ctx, duration, args...)
	if err != nil {
		return fmt.Errorf("extract clip: %w", err)
	}

	return nil
}

// clipEncodeArgs builds the ffmpeg argv for a video clip.
func clipEncodeArgs(
	ffmpegPath, input, output string,
	start, duration float64,
	preset QualityPreset,
	audioIndex int,
	rect CropRect,
) []string {
	return h264EncodeArgs(h264EncodeRequest{
		ffmpegPath: ffmpegPath,
		input:      input,
		output:     output,
		start:      start,
		duration:   duration,
		preset:     preset,
		audioIndex: audioIndex,
		maxWidth:   NormalizeOutputWidth(preset.MaxWidth),
		scaleFlags: scaleFlagsLanczos,
		crop:       rect,
	})
}

// previewEncodeArgs builds the ffmpeg argv for an in-browser preview.
func previewEncodeArgs(
	ffmpegPath, input, output string,
	start, duration float64,
	audioIndex int,
	rect CropRect,
) []string {
	return h264EncodeArgs(h264EncodeRequest{
		ffmpegPath: ffmpegPath,
		input:      input,
		output:     output,
		start:      start,
		duration:   duration,
		preset: QualityPreset{
			CRF:       previewCRF,
			Preset:    previewPreset,
			AudioKbps: previewAudioKbps,
			MaxWidth:  previewMaxWidth,
		},
		audioIndex: audioIndex,
		maxWidth:   previewMaxWidth,
		scaleFlags: scaleFlagsFast,
		crop:       rect,
	})
}

// previewDuration caps a preview window so encodes stay cheap.
func previewDuration(duration float64) float64 {
	if duration < 0 {
		return 0
	}

	return min(duration, previewMaxSecs)
}

// scaleFilter downscales to maxWidth while keeping even dimensions.
func scaleFilter(maxWidth int, flags string) string {
	return fmt.Sprintf("scale=w='trunc(min(%d,iw)/2)*2':h=-2:flags=%s", maxWidth, flags)
}

// videoFilter applies optional black-bar crop then scale.
func videoFilter(maxWidth int, flags string, rect CropRect) string {
	scale := scaleFilter(maxWidth, flags)
	if !rect.Valid() {
		return scale
	}

	return rect.Filter() + "," + scale
}

// h264EncodeArgs builds a browser-safe libx264 argv.
func h264EncodeArgs(req h264EncodeRequest) []string {
	preset := NormalizePreset(req.preset)
	audioIndex := max(req.audioIndex, 0)

	return []string{
		req.ffmpegPath,
		outputFlag,
		ssFlag, formatDuration(req.start),
		inputFlag, req.input,
		durationFlag, formatDuration(req.duration),
		"-map", "0:v:0",
		"-map", "0:a:" + strconv.Itoa(audioIndex) + "?",
		"-c:v", defaultVideoCodec,
		pixelFormatFlag, pixelFormatYUV420P,
		videoFilterFlag, videoFilter(req.maxWidth, req.scaleFlags, req.crop),
		"-crf", strconv.Itoa(preset.CRF),
		"-preset", preset.Preset,
		"-c:a", defaultAudioCodec,
		"-b:a", strconv.Itoa(preset.AudioKbps) + "k",
		"-ac", "2",
		"-movflags", overwriteFlag,
		req.output,
	}
}

// gifPaletteArgs builds the ffmpeg palettegen command.
func gifPaletteArgs(
	ffmpegPath, input, palette string,
	start, duration float64,
	width, fps int,
) []string {
	// Palettegen arguments for the GIF pass.
	return []string{
		ffmpegPath,
		outputFlag,
		ssFlag,
		formatDuration(start),
		inputFlag,
		input,
		durationFlag,
		formatDuration(duration),
		"-vf",
		fmt.Sprintf("fps=%d,scale=%d:-1:flags=lanczos,palettegen=stats_mode=diff", fps, width),
		palette,
	}
}

// gifEncodeArgs builds the ffmpeg paletteuse command.
func gifEncodeArgs(
	ffmpegPath, input, palette, output string,
	start, duration float64,
	width, fps int,
) []string {
	// Paletteuse arguments for the GIF pass.
	return []string{
		ffmpegPath,
		outputFlag,
		ssFlag,
		formatDuration(start),
		inputFlag,
		input,
		inputFlag,
		palette,
		durationFlag,
		formatDuration(duration),
		"-filter_complex",
		fmt.Sprintf(
			"fps=%d,scale=%d:-1:flags=lanczos [x]; [x][1:v] paletteuse=dither=bayer:bayer_scale=5",
			fps,
			width,
		),
		output,
	}
}

// ExtractGIF extracts a GIF from a video.
func (execFFmpeg *ExecFFmpeg) ExtractGIF(
	ctx context.Context,
	input, output string,
	start, duration float64,
	width, fps int,
) error {
	// Build and run the two-pass GIF ffmpeg command.
	cleanInput := filepath.Clean(input)
	cleanOutput := filepath.Clean(output)

	if width <= 0 {
		width = defaultWidth
	}
	if fps <= 0 {
		fps = defaultFPS
	}

	palettePath := cleanOutput + ".palette.png"
	err := execFFmpeg.run(
		ctx,
		duration,
		gifPaletteArgs(
			execFFmpeg.ffmpegPath,
			cleanInput,
			palettePath,
			start,
			duration,
			width,
			fps,
		)...,
	)
	if err != nil {
		return fmt.Errorf("palettegen: %w", err)
	}

	defer osRemove(palettePath)

	gifArgs := gifEncodeArgs(
		execFFmpeg.ffmpegPath,
		cleanInput,
		palettePath,
		cleanOutput,
		start,
		duration,
		width,
		fps,
	)

	err = execFFmpeg.run(ctx, duration, gifArgs...)
	if err != nil {
		return fmt.Errorf("extract GIF: %w", err)
	}

	return nil
}

// ExtractPreview writes a short, downscaled, browser-safe preview segment.
func (execFFmpeg *ExecFFmpeg) ExtractPreview(
	ctx context.Context,
	input, output string,
	start, duration float64,
	audioIndex int,
	rect CropRect,
) error {
	cleanInput := filepath.Clean(input)
	cleanOutput := filepath.Clean(output)

	duration = previewDuration(duration)

	args := previewEncodeArgs(
		execFFmpeg.ffmpegPath,
		cleanInput,
		cleanOutput,
		start,
		duration,
		audioIndex,
		rect,
	)

	err := execFFmpeg.run(ctx, duration, args...)
	if err != nil {
		return fmt.Errorf("extract preview: %w", err)
	}

	return nil
}

// ExtractScreenshot extracts a screenshot from a video.
func (execFFmpeg *ExecFFmpeg) ExtractScreenshot(
	ctx context.Context,
	input, output string,
	timestamp float64,
) error {
	// Build and run the screenshot ffmpeg command.
	cleanInput := filepath.Clean(input)
	cleanOutput := filepath.Clean(output)

	args := []string{
		execFFmpeg.ffmpegPath,
		outputFlag,
		ssFlag, formatDuration(timestamp),
		inputFlag, cleanInput,
		framesFlag, "1",
		qualityFlag, "2",
		cleanOutput,
	}

	err := execFFmpeg.run(ctx, 0, args...)
	if err != nil {
		return fmt.Errorf("extract screenshot: %w", err)
	}

	return nil
}

// Probe probes a media file for information.
func (execFFmpeg *ExecFFmpeg) Probe(ctx context.Context, path string) (MediaInfo, error) {
	cleanPath := filepath.Clean(path)

	args := []string{
		execFFmpeg.ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		cleanPath,
	}

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeoutSec*time.Second)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(probeCtx, args[0], args[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return MediaInfo{}, fmt.Errorf("probe: %w", err)
	}

	result, parseErr := parseProbeOutput(output)
	if parseErr != nil {
		return MediaInfo{}, fmt.Errorf("probe: %w", parseErr)
	}

	return result, nil
}

// SetTimeout sets the FFmpeg timeout.
func (execFFmpeg *ExecFFmpeg) SetTimeout(d time.Duration) {
	execFFmpeg.timeout = d
}

// run executes the FFmpeg command.
func (execFFmpeg *ExecFFmpeg) run(ctx context.Context, duration float64, args ...string) error {
	logging.Logger.Debug().
		Strs("args", args).
		Msg("running ffmpeg command")

	runCtx, cancel := context.WithTimeout(ctx, execFFmpeg.timeout)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(runCtx, args[0], args[1:]...)

	cmd.Dir = "/"

	stderr := progress.NewWriter(duration, progress.From(ctx))

	var stdout bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		logging.Logger.Error().
			Err(err).
			Str("output", stderr.String()+stdout.String()).
			Msg("ffmpeg command failed")

		return fmt.Errorf("ffmpeg: %w", err)
	}

	logging.Logger.Debug().
		Int("exit_code", cmd.ProcessState.ExitCode()).
		Int("output_len", stderr.Len()+stdout.Len()).
		Msg("ffmpeg command completed")

	return nil
}

// formatDuration formats a duration in seconds to a string.
func formatDuration(seconds float64) string {
	return fmt.Sprintf("%.3f", seconds)
}

// osRemove removes a file and logs any errors.
func osRemove(path string) {
	err := os.Remove(path)
	if err != nil {
		logging.Logger.Warn().Str("path", path).Err(err).Msg("failed to remove file")
	}
}
