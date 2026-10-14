// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/crop"
	"github.com/PapagoLabs/outtake/internal/ffmpeg/progress"
	"github.com/PapagoLabs/outtake/internal/logging"
)

const (
	// outputFlag is the FFmpeg overwrite flag.
	outputFlag = "-y"
	// ssFlag is the FFmpeg seek flag.
	ssFlag = "-ss"
	// inputFlag is the FFmpeg input flag.
	inputFlag = "-i"
	// durationFlag is the FFmpeg duration flag.
	durationFlag = "-t"
	// framesFlag is the FFmpeg frames flag.
	framesFlag = "-frames:v"
	// videoFilterFlag is the FFmpeg video-filter flag.
	videoFilterFlag = "-vf"
	// mapFlag selects an input stream for the output.
	mapFlag = "-map"
	// firstVideoStream selects the first input's first video stream.
	firstVideoStream = "0:v:0"
	// videoCodecFlag names the output's video codec.
	videoCodecFlag = "-c:v"
	// anFlag disables audio decoding.
	anFlag = "-an"
	// abortOnFlag names the conditions that make ffmpeg fail instead of finish.
	abortOnFlag = "-abort_on"
	// abortOnEmptyOutput fails a run whose output stream received no packets,
	// which would otherwise finish with an empty file.
	abortOnEmptyOutput = "empty_output_stream"
	// mapMetadataFlag selects where the output's global metadata comes from.
	mapMetadataFlag = "-map_metadata"
	// mapChaptersFlag selects where the output's chapters come from.
	mapChaptersFlag = "-map_chapters"
	// dropAll is the input index that copies nothing.
	dropAll = "-1"
	// threadsFlag bounds a decoder's threads before an input, and an
	// encoder's after the inputs.
	threadsFlag = "-threads"
	// filterThreadsFlag bounds the threads of a simple filter graph.
	filterThreadsFlag = "-filter_threads"
	// filterComplexThreadsFlag bounds the threads of a complex filter graph.
	filterComplexThreadsFlag = "-filter_complex_threads"
	// x265PoolsParam bounds libx265's thread pool, which ignores -threads.
	x265PoolsParam = "pools="
	// x265ParamSeparator joins the parameters in an -x265-params value.
	x265ParamSeparator = ":"
	// commandDir is the working directory for ffmpeg child processes. It is the
	// filesystem root so a child cannot read a relative path out of this
	// process's directory. Media paths passed to a child have to be absolute,
	// which is what mediaPath does, or the child resolves them from here.
	commandDir = "/"
)

// mediaPath cleans a media path and makes it absolute.
//
// ffmpeg children run with commandDir as their working directory, so a relative
// path would be resolved from the filesystem root instead of this process.
//
// Parameters:
//   - path: Media path as the caller named it.
//
// Returns:
//   - abs: The cleaned absolute path.
//   - err: Non-nil when the path cannot be resolved against the working directory.
func mediaPath(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve media path: %w", err)
	}

	return abs, nil
}

// run executes the FFmpeg command within the deadline its clip's length
// allows, reporting progress as it decodes.
//
// Parameters:
//   - ctx: Cancellation and deadline for the command.
//   - duration: Expected duration, used to report progress and set the limit.
//   - args: ffmpeg argv, whose first element is the binary path.
//
// Returns:
//   - err: Non-nil when the process failed.
func (execFFmpeg *ExecFFmpeg) run(
	ctx context.Context,
	duration time.Duration,
	args ...string,
) error {
	//nolint:wrapcheck // runWithin wraps every error it returns.
	return execFFmpeg.runWithin(ctx, duration, execFFmpeg.deadline(duration), args...)
}

// command builds an FFmpeg process with the executor's thread bound.
//
// The child runs in commandDir, and a killed child may hold its output open
// for at most waitDelay.
//
// Parameters:
//   - ctx: Cancellation and deadline for the process.
//   - args: ffmpeg argv, whose first element is the binary path.
//
// Returns:
//   - cmd: The process, not yet started.
func (execFFmpeg *ExecFFmpeg) command(ctx context.Context, args []string) *exec.Cmd {
	args = threaded(args, execFFmpeg.threads)

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	cmd.Dir = commandDir
	cmd.WaitDelay = waitDelay

	return cmd
}

// threaded bounds every thread pool an FFmpeg run starts.
//
// The filter graphs are bounded once, each input's decoder before it, the
// encoders after the last input, and libx265's own pool in its parameters.
//
// Parameters:
//   - args: ffmpeg argv, whose first element is the binary path.
//   - threads: The bound, or zero or less to leave every pool to FFmpeg.
//
// Returns:
//   - bounded: The argv with the bound added.
func threaded(args []string, threads int) []string {
	lastInput := lastInputFlag(args)
	if threads <= 0 || lastInput < 0 {
		return args
	}

	count := strconv.Itoa(threads)

	bounded := make([]string, 0, len(args))

	bounded = append(bounded, args[0], filterThreadsFlag, count, filterComplexThreadsFlag, count)

	for index := 1; index < len(args); index++ {
		if args[index] == inputFlag {
			bounded = append(bounded, threadsFlag, count)
		}

		bounded = append(bounded, x265Pooled(args, index, count))

		if index == lastInput+1 {
			bounded = append(bounded, threadsFlag, count)
		}
	}

	return bounded
}

// lastInputFlag finds the last input flag in an argv.
//
// Parameters:
//   - args: ffmpeg argv.
//
// Returns:
//   - index: The last -i's position, -1 when the argv names no input.
func lastInputFlag(args []string) int {
	for index, arg := range slices.Backward(args) {
		if arg == inputFlag {
			return index
		}
	}

	return -1
}

// x265Pooled returns one argument, with libx265's pool bounded when it is the
// value of -x265-params.
//
// Parameters:
//   - args: ffmpeg argv.
//   - index: The argument to return.
//   - count: The thread bound.
//
// Returns:
//   - arg: The argument, bounded when it holds libx265's parameters.
func x265Pooled(args []string, index int, count string) string {
	if args[index-1] != flagX265Params {
		return args[index]
	}

	return args[index] + x265ParamSeparator + x265PoolsParam + count
}

// runWithin executes the FFmpeg command, reporting progress as it decodes,
// and stops it at limit.
//
// Parameters:
//   - ctx: Cancellation and deadline for the command.
//   - duration: Expected duration, used to report progress.
//   - limit: How long the run may take.
//   - args: ffmpeg argv, whose first element is the binary path.
//
// Returns:
//   - err: Non-nil when the process failed, ErrTimeout when it ran past limit.
func (execFFmpeg *ExecFFmpeg) runWithin(
	ctx context.Context,
	duration, limit time.Duration,
	args ...string,
) error {
	runCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	cmd := execFFmpeg.command(runCtx, args)

	logging.Logger.Debug().
		Strs("args", cmd.Args).
		Msg("running ffmpeg command")

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

		if cmd.ProcessState == nil && runCtx.Err() == nil {
			return fmt.Errorf("%w: %w", ErrNotStarted, err)
		}

		if timedOut(ctx, runCtx) {
			return fmt.Errorf(
				"%w of %s, raise ffmpeg-timeout-sec to allow longer renders",
				ErrTimeout,
				limit,
			)
		}

		return fmt.Errorf("ffmpeg: %w", err)
	}

	logging.Logger.Debug().
		Int("exit_code", cmd.ProcessState.ExitCode()).
		Int("output_len", stderr.Len()+stdout.Len()).
		Msg("ffmpeg command completed")

	return nil
}

// runStderr executes an FFmpeg analysis pass and returns its standard error.
//
// Analysis passes report through stderr rather than through a progress stream,
// so they take the raw text instead.
//
// Parameters:
//   - ctx: Cancellation and deadline for the pass.
//   - label: Pass name, used only for logging.
//   - args: ffmpeg argv, whose first element is the binary path.
//
// Returns:
//   - text: ffmpeg standard error text.
//   - err: The process error, or nil when the pass ran.
func (execFFmpeg *ExecFFmpeg) runStderr(
	ctx context.Context,
	label string,
	args ...string,
) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, execFFmpeg.deadline(0))
	defer cancel()

	cmd := execFFmpeg.command(runCtx, args)

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr != nil {
		logging.Logger.Debug().Err(runErr).Msgf("%s finished", label)
	}

	return stderr.String(), runErr
}

// timedOut reports whether a run was stopped by its own deadline rather than
// by its caller.
//
// Parameters:
//   - caller: The context the run was started under.
//   - run: The run's context, carrying its deadline.
//
// Returns:
//   - timedOut: True when only the run's deadline had passed.
func timedOut(caller, run context.Context) bool {
	return caller.Err() == nil && errors.Is(run.Err(), context.DeadlineExceeded)
}

// prependCrop puts a valid crop filter in front of an ffmpeg filter chain.
//
// Parameters:
//   - rect: Optional black-bar crop.
//   - chain: Filter chain the crop is prepended to.
//
// Returns:
//   - filter: The chain, prefixed with a crop filter when the rectangle is valid.
func prependCrop(rect crop.CropRect, chain string) string {
	if !rect.Valid() {
		return chain
	}

	return rect.Filter() + "," + chain
}
