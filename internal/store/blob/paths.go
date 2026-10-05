// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

import (
	"path/filepath"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// Paths builds the local media paths under a storage base directory.
type Paths struct {
	basePath string
}

// Media directories, relative to the storage base path.
const (
	// Rendered clips live in this directory.
	dirClips = "clips"
	// Rendered GIFs live in this directory.
	dirGifs = "gifs"
	// Rendered previews live in this directory.
	dirPreviews = "previews"
	// Rendered screenshots live in this directory.
	dirScreenshots = "screenshots"
	// Cached Plex thumbnails live in this directory.
	dirThumbnails = "thumbnails"
)

// NewPaths returns the path builder rooted at basePath.
//
// Parameters:
//   - basePath: Storage directory the media lives under.
//
// Returns:
//   - paths: A builder for the layout under basePath.
func NewPaths(basePath string) Paths {
	return Paths{basePath: basePath}
}

// BasePath returns the base storage path.
//
// Returns:
//   - path: The storage directory the layout is rooted at.
func (paths Paths) BasePath() string {
	return paths.basePath
}

// ClipPath returns the path for a clip file.
//
// Parameters:
//   - id: Clip identifier.
//
// Returns:
//   - path: The MP4 path for the clip.
func (paths Paths) ClipPath(id string) string {
	return filepath.Join(paths.ClipsDir(), id+".mp4")
}

// ClipsDir returns the clips directory.
//
// Returns:
//   - dir: The directory holding rendered clips.
func (paths Paths) ClipsDir() string {
	return filepath.Join(paths.basePath, dirClips)
}

// GifPath returns the path for a GIF file.
//
// Parameters:
//   - id: Clip identifier.
//
// Returns:
//   - path: The GIF path for the clip.
func (paths Paths) GifPath(id string) string {
	return filepath.Join(paths.GifsDir(), id+".gif")
}

// GifsDir returns the GIFs directory.
//
// Returns:
//   - dir: The directory holding rendered GIFs.
func (paths Paths) GifsDir() string {
	return filepath.Join(paths.basePath, dirGifs)
}

// OutputPath returns the destination for a produced clip.
//
// Parameters:
//   - id: Clip identifier.
//   - kind: Type of artifact the clip produces.
//
// Returns:
//   - destination: Absolute path, empty for an unrecognized kind.
func (paths Paths) OutputPath(id string, kind clip.Type) string {
	switch kind {
	case clip.TypeClip:
		return paths.ClipPath(id)
	case clip.TypeGIF:
		return paths.GifPath(id)
	case clip.TypeScreenshot:
		return paths.ScreenshotPath(id)
	default:
		return ""
	}
}

// PreviewPath returns the path for a preview file.
//
// Parameters:
//   - id: Media item identifier.
//
// Returns:
//   - path: The MP4 path for the preview.
func (paths Paths) PreviewPath(id string) string {
	return filepath.Join(paths.PreviewsDir(), id+".mp4")
}

// PreviewsDir returns the previews directory.
//
// Returns:
//   - dir: The directory holding rendered previews.
func (paths Paths) PreviewsDir() string {
	return filepath.Join(paths.basePath, dirPreviews)
}

// ScreenshotPath returns the path for a screenshot file.
//
// Parameters:
//   - id: Media item identifier.
//
// Returns:
//   - path: The JPEG path for the screenshot.
func (paths Paths) ScreenshotPath(id string) string {
	return filepath.Join(paths.ScreenshotsDir(), id+".jpg")
}

// ScreenshotsDir returns the screenshots directory.
//
// Returns:
//   - dir: The directory holding rendered screenshots.
func (paths Paths) ScreenshotsDir() string {
	return filepath.Join(paths.basePath, dirScreenshots)
}

// ThumbnailPath returns the path for a thumbnail file.
//
// Parameters:
//   - id: Media item identifier.
//
// Returns:
//   - path: The JPEG path for the thumbnail.
func (paths Paths) ThumbnailPath(id string) string {
	return filepath.Join(paths.ThumbnailsDir(), id+".jpg")
}

// ThumbnailsDir returns the thumbnails directory.
//
// Returns:
//   - dir: The directory holding cached Plex thumbnails.
func (paths Paths) ThumbnailsDir() string {
	return filepath.Join(paths.basePath, dirThumbnails)
}

// mediaDirs lists the directories every backend expects to exist.
//
// Returns:
//   - dirs: The media directory names, relative to the base path.
func mediaDirs() []string {
	return []string{dirClips, dirGifs, dirPreviews, dirScreenshots, dirThumbnails}
}
