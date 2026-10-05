// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package helpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// The ffmpeg tools the suite drives, and the media it drives them with.
const (
	// ffmpegBin encodes the test video and the previews.
	ffmpegBin = "ffmpeg"

	// ffprobeBin probes the media the suite renders from.
	ffprobeBin = "ffprobe"

	// videoName is the generated test video file name.
	videoName = "test-video.mp4"

	// videoPattern is the lavfi source the test video is generated from.
	videoPattern = "testsrc=duration=1:size=160x120:rate=24"

	// VideoSeconds is how long the generated test video runs, which is also the
	// longest clip a spec may ask for from it.
	VideoSeconds = 1

	// clipType is the plain video-clip export type.
	clipType = "clip"

	// movieType is the media type the generated test video is posted as.
	movieType = "movie"

	// qualityLow is the built-in low quality profile id.
	qualityLow = "low"
)

// OnPath reports whether every named binary is on PATH.
//
// Parameters:
//   - names: Binary names to look for.
//
// Returns:
//   - found: True when all of them resolve.
func OnPath(names ...string) bool {
	for _, name := range names {
		_, err := exec.LookPath(name)
		if err != nil {
			return false
		}
	}

	return true
}

// ResolveBinary returns the absolute path of a binary on PATH, or the bare name
// when it cannot be found, which keeps the application configuration readable
// either way.
//
// Parameters:
//   - name: Binary name to resolve.
//
// Returns:
//   - path: Absolute path when resolvable, otherwise name unchanged.
func ResolveBinary(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return name
	}

	return path
}

// GenerateTestVideo writes a one-second test pattern into mediaDir and returns
// its path.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - mediaDir: Directory the video is written to, created when absent.
//
// Returns:
//   - path: Path of the generated video.
//   - err: Wrapped error when the directory or the render fails.
func GenerateTestVideo(ctx context.Context, mediaDir string) (string, error) {
	err := os.MkdirAll(mediaDir, dirPerms)
	if err != nil {
		return "", fmt.Errorf("create the test media directory: %w", err)
	}

	target := filepath.Join(mediaDir, videoName)

	//nolint:gosec // The command is a fixed argument list against the suite's own temp directory.
	cmd := exec.CommandContext(
		ctx,
		ResolveBinary(ffmpegBin),
		"-f", "lavfi",
		"-i", videoPattern,
		"-c:v", "libx264",
		"-t", strconv.Itoa(VideoSeconds),
		"-pix_fmt", "yuv420p",
		"-y",
		target,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("generate the test video: %w: %s", err, string(output))
	}

	return target, nil
}
