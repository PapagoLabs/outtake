// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"uuid"

	"github.com/PapagoLabs/outtake/internal/logging"
)

const (
	// stagingPrefix starts the name of every file a render writes before it is
	// published, and of the GIF palette written beside it.
	stagingPrefix = ".staging-"

	// paletteSuffix ends the name of a GIF palette.
	paletteSuffix = ".palette.png"

	// legacyPaletteSuffix ends the name of a palette written beside a GIF
	// output rather than beside its staging file.
	legacyPaletteSuffix = ".gif" + paletteSuffix
)

var (
	// ErrNoOutputPath reports a render asked to write to no path.
	ErrNoOutputPath = errors.New("no output path")

	// ErrEmptyOutput reports a render that finished without writing anything,
	// or without writing a file at all.
	ErrEmptyOutput = errors.New("ffmpeg wrote an empty file")
)

// SweepStaged removes the staging files and GIF palettes a render left behind
// in each directory, as an interrupted process does. A missing directory is
// skipped.
//
// Parameters:
//   - dirs: Directories renders write to.
//
// Returns:
//   - removed: How many files were removed.
func SweepStaged(dirs ...string) int {
	removed := 0

	for _, dir := range dirs {
		removed += sweepDir(dir)
	}

	return removed
}

// sweepDir removes the staging files and GIF palettes in one directory.
//
// Parameters:
//   - dir: Directory a render writes to.
//
// Returns:
//   - removed: How many files were removed.
func sweepDir(dir string) int {
	removed := 0

	for _, entry := range readRenderDir(dir) {
		if entry.IsDir() || !isLeftover(entry.Name()) {
			continue
		}

		if removeStaged(filepath.Join(dir, entry.Name())) {
			removed++
		}
	}

	return removed
}

// readRenderDir lists a render directory, treating a missing one as empty.
//
// Parameters:
//   - dir: Directory a render writes to.
//
// Returns:
//   - entries: The directory's entries, nil when it cannot be read.
func readRenderDir(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		logging.Logger.Warn().Err(err).Str("dir", dir).Msg("failed to read a render directory")
	}

	return entries
}

// isLeftover reports whether a file name is one a render writes on its way to
// an output.
//
// Parameters:
//   - name: File name without its directory.
//
// Returns:
//   - leftover: True for a staging file, which includes its palette, or a
//     palette left beside a GIF output.
func isLeftover(name string) bool {
	return strings.HasPrefix(name, stagingPrefix) || strings.HasSuffix(name, legacyPaletteSuffix)
}

// outputPath resolves the path a render publishes to. An empty path would
// resolve to the working directory, so it is refused.
//
// Parameters:
//   - path: Destination as the caller named it.
//
// Returns:
//   - abs: The cleaned absolute path.
//   - err: ErrNoOutputPath for an empty path, or the resolve failure.
func outputPath(path string) (string, error) {
	if path == "" {
		return "", ErrNoOutputPath
	}

	abs, err := mediaPath(path)
	if err != nil {
		return "", fmt.Errorf("output: %w", err)
	}

	return abs, nil
}

// publish renders into a staging file beside output and moves it into place
// once the render succeeds, so a failed or canceled render leaves the file
// already at output untouched.
//
// Parameters:
//   - output: Absolute destination path.
//   - render: Writes the file to the staging path it is given.
//
// Returns:
//   - err: The render's error, ErrEmptyOutput when it wrote nothing, or the
//     wrapped stat or rename failure.
func publish(output string, render func(staging string) error) error {
	staging := stagingPath(output)
	defer removeStaged(staging)

	err := render(staging)
	if err != nil {
		//nolint:wrapcheck // The render is the caller's own closure, whose error the caller wraps.
		return err
	}

	info, err := os.Stat(staging)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrEmptyOutput
	}

	if err != nil {
		return fmt.Errorf("stat staged output: %w", err)
	}

	if info.Size() == 0 {
		return ErrEmptyOutput
	}

	err = os.Rename(staging, output)
	if err != nil {
		return fmt.Errorf("publish output: %w", err)
	}

	return nil
}

// stagingPath names the file a render writes before it is published. It sits
// in the output's directory, so the rename that publishes it never crosses a
// filesystem, and it keeps the output's extension, which ffmpeg picks the
// container from.
//
// Parameters:
//   - output: Absolute destination path.
//
// Returns:
//   - staging: A hidden, unique path beside output.
func stagingPath(output string) string {
	return filepath.Join(
		filepath.Dir(output),
		stagingPrefix+uuid.New().String()+filepath.Ext(output),
	)
}

// removeStaged removes a staging file or palette. A file a publish already
// moved away is not an error.
//
// Parameters:
//   - path: The file to remove.
//
// Returns:
//   - removed: True when the file was there and is gone.
func removeStaged(path string) bool {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}

	if err != nil {
		logging.Logger.Warn().Str("path", path).Err(err).Msg("failed to remove a staged file")

		return false
	}

	return true
}
