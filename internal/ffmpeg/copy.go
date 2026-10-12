// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// CopyPlan is how a video clip is copied from its source.
type CopyPlan struct {
	// HEVC reports that the copied video is HEVC, which an MP4 tags hvc1.
	HEVC bool
	// CopyAudio reports that the audio track is copied too, rather than
	// encoded the way an encoded clip's audio is.
	CopyAudio bool
}

const (
	// codecNameH264 is ffprobe's name for H.264 video.
	codecNameH264 = "h264"
	// codecNameHEVC is ffprobe's name for HEVC video.
	codecNameHEVC = "hevc"
	// copiedAudioChannels is the most channels a copied audio track may have,
	// which is what an encoded clip's audio carries.
	copiedAudioChannels = 2
	// copyCodec copies a stream as it is.
	copyCodec = "copy"
	// avoidNegativeTSFlag shifts a copy's timestamps to start at zero.
	avoidNegativeTSFlag = "-avoid_negative_ts"
	// makeZero is the -avoid_negative_ts value that starts the copy at zero.
	makeZero = "make_zero"
	// copyClipErrFmt is the message copy failures are wrapped with.
	copyClipErrFmt = "copy clip: %w"
	// copyLengthTolerance is how far a copy's length may be from the one
	// planned. B-frames and audio priming add a few frames, while a copy that
	// began on another keyframe is off by a whole keyframe interval.
	copyLengthTolerance = 500 * time.Millisecond
	// landingTimeout bounds one read of where a seek lands.
	landingTimeout = 30 * time.Second
	// timebasePrefix opens framecrc's timebase line for the first stream.
	timebasePrefix = "#tb 0:"
	// framecrcPTSField is the field of a framecrc packet line that holds
	// the presentation time.
	framecrcPTSField = 2
	// decodeCheckLength is how much of a copy's start is decoded to check
	// it. Frames that refer to pictures before the cut are all near it.
	decodeCheckLength = 2 * time.Second
	// millisecondsPerSecond converts seconds to milliseconds.
	millisecondsPerSecond = 1000
	// floatBits is the precision ffmpeg's times are parsed with.
	floatBits = 64
	// timebaseSeparator splits a framecrc timebase into its terms.
	timebaseSeparator = "/"
	// fieldSeparator splits a framecrc packet line into its fields.
	fieldSeparator = ","
	// commentPrefix opens a framecrc header line.
	commentPrefix = "#"
)

var (
	// ErrCopyUnavailable reports a clip whose source has no seek point that
	// starts a copy on the keyframe it was planned from.
	ErrCopyUnavailable = errors.New("no seek starts a copy on the keyframe")
	// ErrCopyMismatch reports a copy whose length is not the one planned,
	// so it did not start on the planned keyframe.
	ErrCopyMismatch = errors.New("the copy's length is not the one planned")
	// ErrCopyUnclean reports a copy whose first frames do not decode cleanly,
	// such as one starting on an open GOP keyframe whose leading frames refer
	// to pictures before the cut.
	ErrCopyUnclean = errors.New("the copy does not decode cleanly")
	// errNoLanding reports a seek read that named no packet.
	errNoLanding = errors.New("seek read found no packet")

	// seekPastKeyframe are how far past a keyframe a copy may seek to start
	// on it. Some demuxers, such as Matroska's, land a seek that names the
	// keyframe itself on the keyframe before it.
	seekPastKeyframe = []time.Duration{
		0,
		100 * time.Millisecond,
		200 * time.Millisecond,
		350 * time.Millisecond,
		500 * time.Millisecond,
	}
)

// CopyFits reports whether a video clip can be copied from its source.
//
// A copy keeps the source's video as it is, so the source must already be
// what an encode with the preset would produce. That means no crop, no wider
// than the preset allows, and in the codec and pixel format the encode would
// pick. Those are 8-bit 4:2:0 H.264 for SDR, and 10-bit 4:2:0 HEVC for an HDR
// source the preset keeps HDR for. A Dolby Vision source is never copied.
//
// Parameters:
//   - info: The probed source.
//   - preset: The clip's encode settings.
//   - rect: The black-bar crop detected for the clip.
//   - audioIndex: The clip's audio track.
//
// Returns:
//   - plan: How to copy the clip.
//   - fits: False when the clip has to be encoded.
//
//nolint:nonamedreturns // The named results tell the plan from the flag.
func CopyFits(
	info probe.Info,
	preset clip.QualityPreset,
	rect crop.CropRect,
	audioIndex int,
) (plan CopyPlan, fits bool) {
	if !preset.CopyVideo || rect.Valid() || info.DolbyVision.Present {
		return CopyPlan{}, false
	}

	if info.Width <= 0 || info.Width > clip.NormalizeOutputWidth(preset.MaxWidth) {
		return CopyPlan{}, false
	}

	hevc, fits := copyVideoFits(info, preset)
	if !fits {
		return CopyPlan{}, false
	}

	return CopyPlan{HEVC: hevc, CopyAudio: copyAudioFits(info, audioIndex)}, true
}

// CopyClip copies a clip segment from its source without encoding its video.
//
// The copy starts on keyframe and ends at end. Where a seek lands depends on
// the demuxer, so the seek point is searched for first, and the copy's length
// is checked before it is published. The audio is copied or encoded as plan
// says. Like an encode, the file is written to a staging file and published
// only once it is complete.
//
// Parameters:
//   - ctx: Cancellation and deadline for the copy.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - keyframe: The keyframe the copy starts on, from [ExecFFmpeg.KeyframeBefore].
//   - end: Where the clip ends in the source.
//   - preset: The clip's encode settings, for the audio bitrate.
//   - audioIndex: Audio stream index on the source.
//   - plan: How to copy the clip, from [CopyFits].
//
// Returns:
//   - err: Non-nil when the clip could not be copied. [ErrCopyUnavailable],
//     [ErrCopyMismatch], and [ErrCopyUnclean] mean the clip has to be encoded
//     instead.
func (execFFmpeg *ExecFFmpeg) CopyClip(
	ctx context.Context,
	input, output string,
	keyframe, end time.Duration,
	preset clip.QualityPreset,
	audioIndex int,
	plan CopyPlan,
) error {
	cleanInput, err := mediaPath(input)
	if err != nil {
		return fmt.Errorf(copyClipErrFmt, err)
	}

	cleanOutput, err := outputPath(output)
	if err != nil {
		return fmt.Errorf(copyClipErrFmt, err)
	}

	seek, err := execFFmpeg.seekOnto(ctx, cleanInput, keyframe)
	if err != nil {
		return fmt.Errorf(copyClipErrFmt, err)
	}

	err = publish(ctx, cleanOutput, func(staging string) error {
		return execFFmpeg.copyChecked(ctx, copyArgs(
			execFFmpeg.ffmpegPath, cleanInput, staging, seek, end-seek, preset, audioIndex, plan,
		), staging, end-keyframe)
	})
	if err != nil {
		return fmt.Errorf(copyClipErrFmt, err)
	}

	return nil
}

// KeyframeBefore finds the last keyframe at or before a time in a source.
//
// Parameters:
//   - ctx: Cancellation and deadline for the read.
//   - path: Source media path.
//   - at: The time the keyframe may not be after.
//   - window: How far before at the keyframe may be.
//
// Returns:
//   - keyframe: The keyframe's time, rounded up to the millisecond.
//   - found: False when no keyframe lies in the window.
//   - err: Non-nil when the source cannot be read.
//
//nolint:nonamedreturns // The named results tell the time from the flag.
func (execFFmpeg *ExecFFmpeg) KeyframeBefore(
	ctx context.Context,
	path string,
	at, window time.Duration,
) (keyframe time.Duration, found bool, err error) {
	cleanPath, err := mediaPath(path)
	if err != nil {
		return 0, false, fmt.Errorf("find keyframe: %w", err)
	}

	keyframe, found, err = probe.KeyframeBefore(ctx, execFFmpeg.ffprobePath, cleanPath, at, window)
	if err != nil {
		return 0, false, fmt.Errorf("find keyframe: %w", err)
	}

	return keyframe, found, nil
}

// checkCopyDecodes reports a copy whose start does not decode cleanly.
//
// Parameters:
//   - ctx: Cancellation for the decode.
//   - staging: The copied file.
//
// Returns:
//   - err: [ErrCopyUnclean] when the decoder reports an error, or the run's
//     failure.
func (execFFmpeg *ExecFFmpeg) checkCopyDecodes(ctx context.Context, staging string) error {
	runCtx, cancel := context.WithTimeout(ctx, landingTimeout)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(runCtx, execFFmpeg.ffmpegPath,
		"-v", "error",
		durationFlag, timecode.FromDuration(decodeCheckLength).FormatSeconds(),
		inputFlag, staging,
		"-f", "null",
		"-",
	)

	cmd.Dir = commandDir
	cmd.WaitDelay = waitDelay

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("check copy decodes: %w", err)
	}

	if text := strings.TrimSpace(string(output)); text != "" {
		return fmt.Errorf("%w: %s", ErrCopyUnclean, text)
	}

	return nil
}

// checkCopyLength reports a copy whose length is not the one planned.
//
// Parameters:
//   - ctx: Cancellation for the probe.
//   - staging: The copied file.
//   - want: The planned length.
//
// Returns:
//   - err: [ErrCopyMismatch] when the length is off, or the probe's failure.
func (execFFmpeg *ExecFFmpeg) checkCopyLength(
	ctx context.Context,
	staging string,
	want time.Duration,
) error {
	info, err := probe.Probe(ctx, execFFmpeg.ffprobePath, staging)
	if err != nil {
		return fmt.Errorf("check copy length: %w", err)
	}

	if (info.Duration - want).Abs() > copyLengthTolerance {
		return fmt.Errorf("%w: %s, planned %s", ErrCopyMismatch, info.Duration, want)
	}

	return nil
}

// copyChecked runs a copy and checks its result.
//
// Parameters:
//   - ctx: Cancellation and deadline for the copy.
//   - args: The copy's argv, from copyArgs.
//   - staging: The file the copy writes.
//   - want: The planned length.
//
// Returns:
//   - err: Non-nil when the copy failed or its result is not the one planned.
func (execFFmpeg *ExecFFmpeg) copyChecked(
	ctx context.Context,
	args []string,
	staging string,
	want time.Duration,
) error {
	err := runWithin(ctx, want, execFFmpeg.deadline(want), args...)
	if err != nil {
		return fmt.Errorf("run copy: %w", err)
	}

	err = execFFmpeg.checkCopyLength(ctx, staging, want)
	if err != nil {
		return fmt.Errorf("check copy: %w", err)
	}

	err = execFFmpeg.checkCopyDecodes(ctx, staging)
	if err != nil {
		return fmt.Errorf("check copy: %w", err)
	}

	return nil
}

// seekLanding reports the keyframe a stream copy that seeks to a time starts
// on.
//
// Parameters:
//   - ctx: Cancellation for the read.
//   - input: Source media path.
//   - seek: The time the copy seeks to.
//
// Returns:
//   - landing: The first packet's time, rounded up to the millisecond.
//   - err: Non-nil when the read fails or names no packet.
func (execFFmpeg *ExecFFmpeg) seekLanding(
	ctx context.Context,
	input string,
	seek time.Duration,
) (time.Duration, error) {
	runCtx, cancel := context.WithTimeout(ctx, landingTimeout)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(runCtx, execFFmpeg.ffmpegPath,
		"-v", "error",
		ssFlag, timecode.FromDuration(seek).FormatSeconds(),
		inputFlag, input,
		mapFlag, "0:v:0",
		videoCodecFlag, copyCodec,
		"-copyts",
		framesFlag, "1",
		"-f", "framecrc",
		"-",
	)

	cmd.Dir = commandDir
	cmd.WaitDelay = waitDelay

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("read seek landing: %w", err)
	}

	landing, err := parseLanding(string(output))
	if err != nil {
		return 0, fmt.Errorf("read seek landing: %w", err)
	}

	return landing, nil
}

// seekOnto finds a seek point that starts a stream copy on a keyframe.
//
// Seeking to the keyframe itself is tried first, then points a little past
// it, until a seek lands on the keyframe. A seek that lands past it reached
// the next keyframe, so there is no point trying further.
//
// Parameters:
//   - ctx: Cancellation for the reads.
//   - input: Source media path.
//   - keyframe: The keyframe the copy starts on.
//
// Returns:
//   - seek: The point to seek to.
//   - err: [ErrCopyUnavailable] when no point lands on the keyframe, or the
//     read's failure.
func (execFFmpeg *ExecFFmpeg) seekOnto(
	ctx context.Context,
	input string,
	keyframe time.Duration,
) (time.Duration, error) {
	for _, past := range seekPastKeyframe {
		seek := keyframe + past

		landing, err := execFFmpeg.seekLanding(ctx, input, seek)
		if err != nil {
			return 0, fmt.Errorf("seek onto keyframe: %w", err)
		}

		if (landing - keyframe).Abs() <= time.Millisecond {
			return seek, nil
		}

		if landing > keyframe {
			break
		}
	}

	return 0, ErrCopyUnavailable
}

// copyVideoFits reports whether the source's video is what an encode would
// produce.
//
// Parameters:
//   - info: The probed source.
//   - preset: The clip's encode settings.
//
// Returns:
//   - hevc: True when the copied video is HEVC.
//   - fits: False when the video has to be encoded.
//
//nolint:nonamedreturns // The named results tell the codec from the flag.
func copyVideoFits(info probe.Info, preset clip.QualityPreset) (hevc, fits bool) {
	if clip.IsHDRTransfer(info.ColorTransfer) {
		fits = preset.PreserveHDR &&
			info.VideoCodec == codecNameHEVC &&
			info.PixelFormat == pixelFormatYUV420P10LE

		return fits, fits
	}

	return false, info.VideoCodec == codecNameH264 && info.PixelFormat == pixelFormatYUV420P
}

// copyAudioFits reports whether the clip's audio track is what an encode
// would produce, AAC with at most two channels.
//
// Parameters:
//   - info: The probed source.
//   - audioIndex: The clip's audio track.
//
// Returns:
//   - fits: False when the audio has to be encoded.
func copyAudioFits(info probe.Info, audioIndex int) bool {
	if audioIndex < 0 || audioIndex >= len(info.AudioTracks) {
		return false
	}

	track := info.AudioTracks[audioIndex]

	return track.Codec == defaultAudioCodec && track.Channels > 0 &&
		track.Channels <= copiedAudioChannels
}

// copyArgs builds the argv of a copy.
//
// Parameters:
//   - ffmpegPath: Path to the ffmpeg binary.
//   - input: Source media path.
//   - output: Destination mp4 path.
//   - start: The keyframe the copy starts on.
//   - duration: Length of the clip from start.
//   - preset: The clip's encode settings, for the audio bitrate.
//   - audioIndex: Audio stream index on the source.
//   - plan: How to copy the clip.
//
// Returns:
//   - args: ffmpeg argv including the binary path.
func copyArgs(
	ffmpegPath, input, output string,
	start, duration time.Duration,
	preset clip.QualityPreset,
	audioIndex int,
	plan CopyPlan,
) []string {
	args := []string{
		ffmpegPath,
		outputFlag,
		abortOnFlag, abortOnEmptyOutput,
		ssFlag, timecode.FromDuration(start).FormatSeconds(),
		inputFlag, input,
		durationFlag, timecode.FromDuration(duration).FormatSeconds(),
		mapFlag, "0:v:0",
		mapFlag, "0:a:" + strconv.Itoa(max(audioIndex, 0)) + "?",
		videoCodecFlag, copyCodec,
	}

	if plan.HEVC {
		args = append(args, "-tag:v", hevcTag)
	}

	if plan.CopyAudio {
		args = append(args, "-c:a", copyCodec)
	} else {
		args = append(args,
			"-c:a", defaultAudioCodec,
			"-b:a", strconv.Itoa(clip.NormalizePreset(preset).AudioKbps)+"k",
			"-ac", strconv.Itoa(copiedAudioChannels),
		)
	}

	// The source's title, tags, and chapters describe the whole source, not
	// the clip, so none of them is copied into it.
	args = append(args,
		mapMetadataFlag, dropAll, mapChaptersFlag, dropAll,
		avoidNegativeTSFlag, makeZero,
		// write_colr carries the source's color tags into the MP4.
		"-movflags", toneMappedMovFlags,
		output,
	)

	return args
}

// parseLanding reads the first packet's time from framecrc output.
//
// Parameters:
//   - text: framecrc output for the first stream.
//
// Returns:
//   - landing: The packet's time, rounded up to the millisecond.
//   - err: [errNoLanding] when the output names no timebase or packet.
func parseLanding(text string) (time.Duration, error) {
	var timebase float64

	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)

		if rest, ok := strings.CutPrefix(line, timebasePrefix); ok {
			timebase = parseTimebase(rest)

			continue
		}

		if !packetLine(line, timebase) {
			continue
		}

		at, err := packetTime(line, timebase)
		if err != nil {
			return 0, fmt.Errorf("read packet time: %w", err)
		}

		return at, nil
	}

	return 0, errNoLanding
}

// parseTimebase reads a framecrc timebase.
//
// Parameters:
//   - text: The timebase, such as 1/1000.
//
// Returns:
//   - seconds: The length of one tick, 0 when the timebase cannot be read.
func parseTimebase(text string) float64 {
	num, den, found := strings.Cut(strings.TrimSpace(text), timebaseSeparator)
	if !found {
		return 0
	}

	numerator, err := strconv.ParseFloat(num, floatBits)
	if err != nil {
		return 0
	}

	denominator, err := strconv.ParseFloat(den, floatBits)
	if err != nil || denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// packetTime reads a framecrc packet line's presentation time.
//
// Parameters:
//   - line: The packet line.
//   - timebase: The length of one tick, in seconds.
//
// Returns:
//   - at: The packet's time, rounded up to the millisecond.
//   - err: [errNoLanding] when the line holds no time.
func packetTime(line string, timebase float64) (time.Duration, error) {
	fields := strings.Split(line, fieldSeparator)
	if len(fields) <= framecrcPTSField {
		return 0, errNoLanding
	}

	pts, err := strconv.ParseFloat(strings.TrimSpace(fields[framecrcPTSField]), floatBits)
	if err != nil {
		return 0, errNoLanding
	}

	return time.Duration(math.Ceil(pts*timebase*millisecondsPerSecond)) * time.Millisecond, nil
}

// packetLine reports whether a framecrc line is a packet that can be read.
//
// Parameters:
//   - line: A trimmed framecrc line.
//   - timebase: The timebase read so far, 0 before one is read.
//
// Returns:
//   - ok: False for a blank or header line, or before the timebase.
func packetLine(line string, timebase float64) bool {
	return line != "" && !strings.HasPrefix(line, commentPrefix) && timebase != 0
}
