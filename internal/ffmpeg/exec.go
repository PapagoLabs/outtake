// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	// anFlag disables audio decoding.
	anFlag = "-an"
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

// run executes the FFmpeg command, reporting progress as it decodes.
//
// Parameters:
//   - ctx: Cancellation and deadline for the command.
//   - duration: Expected duration, used to report progress.
//   - args: ffmpeg argv, whose first element is the binary path.
//
// Returns:
//   - err: Non-nil when the process failed.
func (execFFmpeg *ExecFFmpeg) run(
	ctx context.Context,
	duration time.Duration,
	args ...string,
) error {
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
	runCtx, cancel := context.WithTimeout(ctx, execFFmpeg.timeout)
	defer cancel()

	// #nosec G204 - args are controlled by the application
	cmd := exec.CommandContext(runCtx, args[0], args[1:]...)

	cmd.Dir = commandDir

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr != nil {
		logging.Logger.Debug().Err(runErr).Msgf("%s finished", label)
	}

	return stderr.String(), runErr
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

// osRemove removes a file and logs any errors.
//
// Parameters:
//   - path: File to remove.
func osRemove(path string) {
	err := os.Remove(path)
	if err != nil {
		logging.Logger.Warn().Str("path", path).Err(err).Msg("failed to remove file")
	}
}
