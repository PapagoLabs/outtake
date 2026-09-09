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
	"github.com/PapagoLabs/outtake/internal/media/probe"
	"github.com/PapagoLabs/outtake/internal/media/progress"
	"github.com/PapagoLabs/outtake/internal/media/websafe"
)

// ExecFFmpeg provides FFmpeg execution capabilities.
type ExecFFmpeg struct {
	ffmpegPath  string
	ffprobePath string
	timeout     time.Duration
}

// h264EncodeRequest is the input for a browser-safe libx264 encode.
type h264EncodeRequest struct {
	ffmpegPath   string
	input        string
	output       string
	start        float64
	duration     float64
	preset       QualityPreset
	audioIndex   int
	maxWidth     int
	scaleFlags   string
	crop         CropRect
	webSafeColor bool
	hdrKind      string
	tonePeak     float64
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
	// CommandDir is the working directory for ffmpeg child processes.
	commandDir = "/"
	// WebSafeMovFlags writes a colr atom so browsers agree on Rec.709.
	webSafeMovFlags = "+faststart+write_colr"
	// WebSafePeakSecs caps how long a luma-peak pass may sample.
	webSafePeakSecs = 8
	// SignalstatsFilter prints lavfi.signalstats.YMAX to stderr for peak detect.
	signalstatsFilter = "signalstats,metadata=mode=print"
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
	// AnFlag disables audio decoding.
	anFlag = "-an"
	// UpdateFlag tells image2 to overwrite a single still.
	updateFlag = "-update"
	// UpdateEnabled is the image2 single-file update value.
	updateEnabled = "1"
	// FilterComplexFlag is the FFmpeg filter_complex flag.
	filterComplexFlag = "-filter_complex"
	// GIFScaleHeight keeps GIF scale height even.
	gifScaleHeight = "-2"
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

	cmd.Dir = commandDir

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

	req := clipEncodeRequest(
		execFFmpeg.ffmpegPath,
		cleanInput,
		cleanOutput,
		start,
		duration,
		preset,
		audioIndex,
		rect,
	)
	if req.webSafeColor {
		execFFmpeg.applyWebSafe(ctx, &req)
	}

	err := execFFmpeg.run(ctx, duration, h264EncodeArgs(req)...)
	if err != nil {
		return fmt.Errorf("extract clip: %w", err)
	}

	return nil
}

// clipEncodeArgs builds the ffmpeg argv for a video clip.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset in seconds.
//   - duration: Clip duration in seconds.
//   - preset: Encode quality, including WebSafeColor.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func clipEncodeArgs(
	ffmpegPath, input, output string,
	start, duration float64,
	preset QualityPreset,
	audioIndex int,
	rect CropRect,
) []string {
	return h264EncodeArgs(clipEncodeRequest(
		ffmpegPath,
		input,
		output,
		start,
		duration,
		preset,
		audioIndex,
		rect,
	))
}

// clipEncodeRequest builds the shared H.264 encode request for a saved clip.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset in seconds.
//   - duration: Clip duration in seconds.
//   - preset: Encode quality, including WebSafeColor.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - req: Populated encode request. HDR peak is filled later by applyWebSafe.
func clipEncodeRequest(
	ffmpegPath, input, output string,
	start, duration float64,
	preset QualityPreset,
	audioIndex int,
	rect CropRect,
) h264EncodeRequest {
	hdrKind := ""
	if preset.WebSafeColor {
		hdrKind = websafe.TransferPQAlias
	}

	return h264EncodeRequest{
		ffmpegPath:   ffmpegPath,
		input:        input,
		output:       output,
		start:        start,
		duration:     duration,
		preset:       preset,
		audioIndex:   audioIndex,
		maxWidth:     NormalizeOutputWidth(preset.MaxWidth),
		scaleFlags:   scaleFlagsLanczos,
		crop:         rect,
		webSafeColor: preset.WebSafeColor,
		hdrKind:      hdrKind,
		tonePeak:     websafe.DefaultPeak,
	}
}

// previewEncodeArgs builds the ffmpeg argv for an in-browser preview.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset in seconds.
//   - duration: Preview duration in seconds.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//   - preset: Encode options; only WebSafeColor is read.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func previewEncodeArgs(
	ffmpegPath, input, output string,
	start, duration float64,
	audioIndex int,
	rect CropRect,
	preset QualityPreset,
) []string {
	return h264EncodeArgs(previewEncodeRequest(
		ffmpegPath,
		input,
		output,
		start,
		duration,
		audioIndex,
		rect,
		preset,
	))
}

// previewEncodeRequest builds the shared H.264 encode request for a preview.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: Seek offset in seconds.
//   - duration: Preview duration in seconds.
//   - audioIndex: Audio stream index on the source.
//   - rect: Optional black-bar crop.
//   - preset: Encode options; only WebSafeColor is read.
//
// Returns:
//   - req: Populated encode request. HDR peak is filled later by applyWebSafe.
func previewEncodeRequest(
	ffmpegPath, input, output string,
	start, duration float64,
	audioIndex int,
	rect CropRect,
	preset QualityPreset,
) h264EncodeRequest {
	hdrKind := ""
	if preset.WebSafeColor {
		hdrKind = websafe.TransferPQAlias
	}

	return h264EncodeRequest{
		ffmpegPath: ffmpegPath,
		input:      input,
		output:     output,
		start:      start,
		duration:   duration,
		preset: QualityPreset{
			CRF:          previewCRF,
			Preset:       previewPreset,
			AudioKbps:    previewAudioKbps,
			MaxWidth:     previewMaxWidth,
			WebSafeColor: preset.WebSafeColor,
		},
		audioIndex:   audioIndex,
		maxWidth:     previewMaxWidth,
		scaleFlags:   scaleFlagsFast,
		crop:         rect,
		webSafeColor: preset.WebSafeColor,
		hdrKind:      hdrKind,
		tonePeak:     websafe.DefaultPeak,
	}
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

// prependCrop puts a valid crop filter in front of an ffmpeg filter chain.
func prependCrop(rect CropRect, chain string) string {
	if !rect.Valid() {
		return chain
	}

	return rect.Filter() + "," + chain
}

// videoFilter applies optional black-bar crop, optional HDR tone-map, then scale.
//
// Parameters:
//   - req: Encode request with crop, scale, and web-safe color fields.
//
// Returns:
//   - filter: The ffmpeg -vf chain.
func videoFilter(req h264EncodeRequest) string {
	chain := scaleFilter(req.maxWidth, req.scaleFlags)
	if req.webSafeColor && req.hdrKind != "" {
		chain = websafe.ToneMapFilter(req.hdrKind, req.tonePeak) + "," + chain
	}

	return prependCrop(req.crop, chain)
}

// h264EncodeArgs builds a browser-safe libx264 argv.
//
// Parameters:
//   - req: Encode request for a clip or preview.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func h264EncodeArgs(req h264EncodeRequest) []string {
	preset := NormalizePreset(req.preset)
	audioIndex := max(req.audioIndex, 0)

	args := []string{
		req.ffmpegPath,
		outputFlag,
		ssFlag, formatDuration(req.start),
		inputFlag, req.input,
		durationFlag, formatDuration(req.duration),
		"-map", "0:v:0",
		"-map", "0:a:" + strconv.Itoa(audioIndex) + "?",
		"-c:v", defaultVideoCodec,
		pixelFormatFlag, pixelFormatYUV420P,
		videoFilterFlag, videoFilter(req),
		"-crf", strconv.Itoa(preset.CRF),
		"-preset", preset.Preset,
		"-c:a", defaultAudioCodec,
		"-b:a", strconv.Itoa(preset.AudioKbps) + "k",
		"-ac", "2",
	}
	if req.webSafeColor {
		args = append(args, webSafeColorArgs()...)
	}

	return append(args, "-movflags", movFlags(req), req.output)
}

// webSafeColorArgs tags the encode as Rec.709 / sRGB limited range.
//
// Returns:
//   - args: ffmpeg color and x264-params flags.
func webSafeColorArgs() []string {
	return []string{
		"-color_primaries", "bt709",
		"-color_trc", "iec61966-2-1",
		"-colorspace", "bt709",
		"-color_range", "tv",
		"-x264-params", "colorprim=bt709:transfer=iec61966-2-1:colormatrix=bt709",
	}
}

// movFlags returns container flags, adding colr when web-safe color is on.
//
// Parameters:
//   - req: Encode request whose WebSafeColor field selects the movflags value.
//
// Returns:
//   - flags: +faststart, or +faststart+write_colr when web-safe color is on.
func movFlags(req h264EncodeRequest) string {
	if req.webSafeColor {
		return webSafeMovFlags
	}

	return overwriteFlag
}

// gifScaleFilter is the shared fps+scale chain for GIF palette and encode.
//
// Parameters:
//   - width: Output width in pixels.
//   - fps: Output frames per second.
//
// Returns:
//   - filter: fps and even-height scale fragment.
func gifScaleFilter(width, fps int) string {
	return fmt.Sprintf("fps=%d,scale=%d:%s:flags=lanczos", fps, width, gifScaleHeight)
}

// gifVideoChain is the shared decode/scale chain for both GIF passes.
//
// Parameters:
//   - width: Output width in pixels.
//   - fps: Output frames per second.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - filter: crop, fps, scale, and yuv420p conversion.
func gifVideoChain(width, fps int, rect CropRect) string {
	return prependCrop(rect, gifScaleFilter(width, fps)+",format=yuv420p")
}

// gifPaletteFilter builds the palettegen -vf chain, with optional crop first.
//
// Parameters:
//   - width: Output width in pixels.
//   - fps: Output frames per second.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - filter: Palette generation -vf string.
func gifPaletteFilter(width, fps int, rect CropRect) string {
	return gifVideoChain(width, fps, rect) + ",palettegen=stats_mode=diff"
}

// gifEncodeFilter builds the paletteuse -filter_complex chain.
//
// Parameters:
//   - width: Output width in pixels.
//   - fps: Output frames per second.
//   - rect: Optional black-bar crop.
//
// Returns:
//   - filter: Labeled paletteuse filter_complex string.
func gifEncodeFilter(width, fps int, rect CropRect) string {
	return "[0:v]" + gifVideoChain(width, fps, rect) +
		"[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5"
}

// gifSeekArgs prefixes ffmpeg with an input-limited seek.
//
// -t must come before -i so palettegen does not decode to EOF.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - start: Seek offset in seconds.
//   - duration: Input duration in seconds.
//
// Returns:
//   - args: argv through the first -i inclusive.
func gifSeekArgs(ffmpegPath, input string, start, duration float64) []string {
	return []string{
		ffmpegPath,
		outputFlag,
		ssFlag,
		formatDuration(start),
		durationFlag,
		formatDuration(duration),
		inputFlag,
		input,
	}
}

// gifPaletteArgs builds the ffmpeg palettegen command.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - palette: Destination palette PNG path.
//   - start: Seek offset in seconds.
//   - duration: Input duration in seconds.
//   - vf: palettegen -vf chain.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func gifPaletteArgs(
	ffmpegPath, input, palette string,
	start, duration float64,
	vf string,
) []string {
	args := gifSeekArgs(ffmpegPath, input, start, duration)

	args = append(
		args,
		anFlag,
		videoFilterFlag,
		vf,
		framesFlag,
		"1",
		updateFlag,
		updateEnabled,
		palette,
	)

	return args
}

// gifEncodeArgs builds the ffmpeg paletteuse command.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - palette: Palette PNG path.
//   - output: Destination GIF path.
//   - start: Seek offset in seconds.
//   - duration: Input duration in seconds.
//   - filter: paletteuse filter_complex string.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func gifEncodeArgs(
	ffmpegPath, input, palette, output string,
	start, duration float64,
	filter string,
) []string {
	args := gifSeekArgs(ffmpegPath, input, start, duration)

	args = append(args, inputFlag, palette, anFlag, filterComplexFlag, filter, output)

	return args
}

// ExtractGIF extracts a GIF from a video.
func (execFFmpeg *ExecFFmpeg) ExtractGIF(
	ctx context.Context,
	input, output string,
	start, duration float64,
	width, fps int,
	rect CropRect,
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
			gifPaletteFilter(width, fps, rect),
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
		gifEncodeFilter(width, fps, rect),
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
	preset QualityPreset,
) error {
	cleanInput := filepath.Clean(input)
	cleanOutput := filepath.Clean(output)

	duration = previewDuration(duration)

	req := previewEncodeRequest(
		execFFmpeg.ffmpegPath,
		cleanInput,
		cleanOutput,
		start,
		duration,
		audioIndex,
		rect,
		preset,
	)
	if req.webSafeColor {
		execFFmpeg.applyWebSafe(ctx, &req)
	}

	err := execFFmpeg.run(ctx, duration, h264EncodeArgs(req)...)
	if err != nil {
		return fmt.Errorf("extract preview: %w", err)
	}

	return nil
}

// screenshotEncodeArgs builds the ffmpeg argv for a still frame.
func screenshotEncodeArgs(
	ffmpegPath, input, output string,
	timestamp float64,
	rect CropRect,
) []string {
	args := []string{
		ffmpegPath,
		outputFlag,
		ssFlag, formatDuration(timestamp),
		inputFlag, input,
		framesFlag, "1",
		qualityFlag, "2",
	}
	if rect.Valid() {
		args = append(args, videoFilterFlag, rect.Filter())
	}

	return append(args, output)
}

// ExtractScreenshot extracts a screenshot from a video.
func (execFFmpeg *ExecFFmpeg) ExtractScreenshot(
	ctx context.Context,
	input, output string,
	timestamp float64,
	rect CropRect,
) error {
	// Build and run the screenshot ffmpeg command.
	cleanInput := filepath.Clean(input)
	cleanOutput := filepath.Clean(output)

	err := execFFmpeg.run(
		ctx,
		0,
		screenshotEncodeArgs(execFFmpeg.ffmpegPath, cleanInput, cleanOutput, timestamp, rect)...,
	)
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

	result, parseErr := probe.ParseOutput(output)
	if parseErr != nil {
		return MediaInfo{}, fmt.Errorf("probe: %w", parseErr)
	}

	return result, nil
}

// SetTimeout sets the FFmpeg timeout.
func (execFFmpeg *ExecFFmpeg) SetTimeout(d time.Duration) {
	execFFmpeg.timeout = d
}

// applyWebSafe sets CPU tone-map parameters from the source stream.
//
// SDR sources keep Rec.709 tags only. Probe or peak failures leave tags
// without a remaster.
//
// Parameters:
//   - ctx: Cancellation and deadline for probe and luma sampling.
//   - req: Encode request to update in place.
func (execFFmpeg *ExecFFmpeg) applyWebSafe(ctx context.Context, req *h264EncodeRequest) {
	info, err := execFFmpeg.Probe(ctx, req.input)
	if err != nil {
		req.hdrKind = ""

		return
	}

	if !websafe.IsHDRTransfer(info.ColorTransfer) {
		req.hdrKind = ""

		return
	}

	if websafe.IsHLGTransfer(info.ColorTransfer) {
		req.hdrKind = websafe.TransferHLGAlias
		req.tonePeak = websafe.DefaultPeak

		return
	}

	req.hdrKind = websafe.TransferPQAlias

	ymax, ok := execFFmpeg.signalstatsYMax(ctx, req.input, req.start, req.duration)
	if !ok {
		return
	}

	req.tonePeak = websafe.TonePeakFromNits(websafe.PQNitsFromLimitedY(ymax))
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

	cmd.Dir = commandDir

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

// signalstatsYMax samples luma on a short window of the clip.
//
// Parameters:
//   - ctx: Cancellation and deadline for the ffmpeg pass.
//   - input: Source media path.
//   - start: Seek offset in seconds.
//   - duration: Clip duration in seconds; capped at webSafePeakSecs.
//
// Returns:
//   - ymax: Highest limited-range luma code observed.
//   - ok: True when at least one YMAX value was parsed.
func (execFFmpeg *ExecFFmpeg) signalstatsYMax(
	ctx context.Context,
	input string,
	start, duration float64,
) (float64, bool) {
	sample := duration
	if sample <= 0 || sample > webSafePeakSecs {
		sample = webSafePeakSecs
	}

	args := []string{
		execFFmpeg.ffmpegPath,
		ssFlag, formatDuration(start),
		inputFlag, input,
		durationFlag, formatDuration(sample),
		"-an",
		videoFilterFlag, signalstatsFilter,
		"-f", "null",
		"-",
	}

	runCtx, cancel := context.WithTimeout(ctx, execFFmpeg.timeout)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(runCtx, args[0], args[1:]...)

	cmd.Dir = commandDir

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logging.Logger.Debug().Err(err).Msg("signalstats finished")
	}

	return websafe.ParseSignalstatsYMax(stderr.String())
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
