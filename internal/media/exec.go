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
)

// ExecFFmpeg provides FFmpeg execution capabilities.
type ExecFFmpeg struct {
	ffmpegPath  string
	ffprobePath string
	timeout     time.Duration
}

const (
	// DefaultFPS is the default frames per second.
	defaultFPS = 10
	// DefaultWidth is the default width.
	defaultWidth = 480
	// DefaultDuration is the default audio bitrate.
	defaultDuration = "128k"
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
)

// NewExecFFmpeg creates a new FFmpeg executor.
func NewExecFFmpeg(ffmpegPath, ffprobePath string) *ExecFFmpeg {
	return &ExecFFmpeg{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		timeout:     DefaultFFmpegTimeout(),
	}
}

// ExtractClip extracts a clip from a video.
func (execFFmpeg *ExecFFmpeg) ExtractClip(
	ctx context.Context,
	input, output string,
	start, duration float64,
	quality ClipQuality,
) error {
	// Build and run the clip ffmpeg command.
	cleanInput := filepath.Clean(input)
	cleanOutput := filepath.Clean(output)

	preset, ok := QualityPresets[quality]
	if !ok {
		preset = QualityPresets[ClipQualityMedium]
	}

	args := []string{
		execFFmpeg.ffmpegPath,
		outputFlag,
		ssFlag, formatDuration(start),
		inputFlag, cleanInput,
		durationFlag, formatDuration(duration),
		"-c:v", defaultVideoCodec,
		"-crf", strconv.Itoa(preset.CRF),
		"-preset", preset.Preset,
		"-c:a", defaultAudioCodec,
		"-b:a", defaultDuration,
		"-movflags", overwriteFlag,
		cleanOutput,
	}

	err := execFFmpeg.run(ctx, duration, args...)
	if err != nil {
		return fmt.Errorf("extract clip: %w", err)
	}

	return nil
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

	stderr := &progressWriter{
		duration: duration,
		on:       progressFrom(ctx),
		buf:      bytes.Buffer{},
	}
	var stdout bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		logging.Logger.Error().
			Err(err).
			Str("output", stderr.buf.String()+stdout.String()).
			Msg("ffmpeg command failed")

		return fmt.Errorf("ffmpeg: %w", err)
	}

	logging.Logger.Debug().
		Int("exit_code", cmd.ProcessState.ExitCode()).
		Int("output_len", stderr.buf.Len()+stdout.Len()).
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
