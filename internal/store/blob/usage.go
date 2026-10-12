// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// FolderUsage is how many files one storage folder holds, and their size.
type FolderUsage struct {
	// Files is how many files the folder holds.
	Files int
	// Bytes is their total size.
	Bytes int64
}

// Usage is what the local storage holds, folder by folder.
type Usage struct {
	// Clips is the video clips, with their SDR versions.
	Clips FolderUsage
	// GIFs is the GIF clips.
	GIFs FolderUsage
	// Screenshots is the screenshot clips.
	Screenshots FolderUsage
	// Previews is the export form's previews.
	Previews FolderUsage
	// Thumbnails is the poster cache.
	Thumbnails FolderUsage
}

// FileDeleter deletes one stored file, which is all a clear needs of a
// [Blob].
type FileDeleter interface {
	// DeleteFile deletes the object identified by path.
	DeleteFile(path string) error
}

// deletion collects what the deletes DeleteEach runs come to.
type deletion struct {
	// first is the first delete that failed, wrapped with its file's name.
	first error
	// once guards first.
	once sync.Once
	// removed counts the files deleted.
	removed atomic.Int64
}

const (
	// stagingPrefix opens the name of a file a render is still writing.
	stagingPrefix = ".staging-"
	// paletteSuffix ends the name of a GIF palette a render writes.
	paletteSuffix = ".palette.png"
	// ClearLimit is how many deletes a clear runs at once. On S3 each is a
	// request.
	ClearLimit = 4
)

// Total returns what every folder holds together.
//
// Returns:
//   - total: The sum of every folder.
func (usage Usage) Total() FolderUsage {
	total := FolderUsage{Files: 0, Bytes: 0}

	for _, folder := range []FolderUsage{usage.Clips, usage.GIFs, usage.Screenshots, usage.Previews, usage.Thumbnails} {
		total.Files += folder.Files
		total.Bytes += folder.Bytes
	}

	return total
}

// Usage measures what the local storage holds.
//
// Only finished files count. A render's staging file or GIF palette is left
// out, and a folder nothing has written yet holds nothing. A folder that
// cannot be read reads as empty, and the others are still measured.
//
// Returns:
//   - usage: What each readable folder holds.
//   - err: Every folder that cannot be read, or nil.
func (paths Paths) Usage() (Usage, error) {
	var (
		usage  Usage
		failed []error
	)

	for _, folder := range []struct {
		dir   string
		usage *FolderUsage
	}{
		{dir: paths.ClipsDir(), usage: &usage.Clips},
		{dir: paths.GifsDir(), usage: &usage.GIFs},
		{dir: paths.ScreenshotsDir(), usage: &usage.Screenshots},
		{dir: paths.PreviewsDir(), usage: &usage.Previews},
		{dir: paths.ThumbnailsDir(), usage: &usage.Thumbnails},
	} {
		measured, err := folderUsage(folder.dir)
		if err != nil {
			failed = append(failed, err)

			continue
		}

		*folder.usage = measured
	}

	err := errors.Join(failed...)
	if err != nil {
		return usage, fmt.Errorf("measure storage: %w", err)
	}

	return usage, nil
}

// StoredFiles lists the finished files in one storage folder.
//
// Parameters:
//   - dir: The folder.
//
// Returns:
//   - files: Each finished file's path, empty for a folder nothing has
//     written yet.
//   - err: Non-nil when the folder cannot be read.
func StoredFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}

	files := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.Type().IsRegular() && finished(entry.Name()) {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}

// ClearThumbnails deletes the cached posters, a few at a time.
//
// The cache refills from Plex as pages ask for posters again. On S3 each
// poster's object goes too. A delete that fails does not stop the others.
//
// Parameters:
//   - ctx: Request scope. Once it ends no further delete starts.
//   - store: The storage the posters are deleted from.
//   - paths: The local layout the posters are listed from.
//
// Returns:
//   - removed: How many posters were deleted.
//   - err: The first failure, or nil.
func ClearThumbnails(ctx context.Context, store FileDeleter, paths Paths) (int, error) {
	files, err := StoredFiles(paths.ThumbnailsDir())
	if err != nil {
		return 0, fmt.Errorf("clear thumbnails: %w", err)
	}

	removed, err := DeleteEach(ctx, store, files, ClearLimit)
	if err != nil {
		return removed, fmt.Errorf("clear thumbnails: %w", err)
	}

	return removed, nil
}

// DeleteEach deletes files, at most limit at a time.
//
// A delete that fails does not stop the others, and once ctx ends no further
// delete starts.
//
// Parameters:
//   - ctx: Request scope for the deletes.
//   - store: The storage the files are deleted from.
//   - files: The files to delete.
//   - limit: How many deletes may run at once, at least one.
//
// Returns:
//   - removed: How many files were deleted.
//   - err: The first failure, or nil.
func DeleteEach(ctx context.Context, store FileDeleter, files []string, limit int) (int, error) {
	var (
		run   deletion
		group sync.WaitGroup
	)

	slots := make(chan struct{}, max(limit, 1))

	for _, file := range files {
		if !takeSlot(ctx, slots) {
			break
		}

		group.Go(func() {
			defer func() { <-slots }()

			run.remove(store, file)
		})
	}

	group.Wait()

	return int(run.removed.Load()), run.first
}

// remove deletes one file, counting it or recording the first failure.
//
// Parameters:
//   - store: The storage the file is deleted from.
//   - file: The file to delete.
func (run *deletion) remove(store FileDeleter, file string) {
	err := store.DeleteFile(file)
	if err != nil {
		run.once.Do(func() { run.first = fmt.Errorf("delete %s: %w", filepath.Base(file), err) })

		return
	}

	run.removed.Add(1)
}

// folderUsage measures one storage folder.
//
// Parameters:
//   - dir: The folder.
//
// Returns:
//   - usage: How many finished files it holds, and their size.
//   - err: Non-nil when the folder cannot be read.
func folderUsage(dir string) (FolderUsage, error) {
	files, err := StoredFiles(dir)
	if err != nil {
		return FolderUsage{}, fmt.Errorf("measure %s: %w", filepath.Base(dir), err)
	}

	usage := FolderUsage{Files: 0, Bytes: 0}

	for _, file := range files {
		info, statErr := os.Stat(file)
		if statErr != nil {
			// A file deleted while the folder was read holds nothing.
			continue
		}

		usage.Files++

		usage.Bytes += info.Size()
	}

	return usage, nil
}

// finished reports whether a file name is a finished file rather than one a
// render is still writing.
//
// Parameters:
//   - name: The file's name.
//
// Returns:
//   - ok: False for a staging file or a GIF palette.
func finished(name string) bool {
	return !strings.HasPrefix(name, stagingPrefix) && !strings.HasSuffix(name, paletteSuffix)
}
