// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/clip/storage"
)

const (
	// DefaultQuality is the built-in medium clip profile id.
	DefaultQuality = "medium"
	// DefaultMediaType is the default Plex media type for new clips.
	DefaultMediaType = "movie"
)

// NormalizeClipType maps API clip type aliases onto queue job types.
//
// Parameters:
//   - clipType: Requested type, including the legacy "video" alias.
//
// Returns:
//   - jobType: Canonical queue job type.
//   - ok: False when the type is not recognized.
func NormalizeClipType(clipType string) (queue.JobType, bool) {
	switch clipType {
	case "clip", "video":
		return queue.JobTypeClip, true
	case "gif":
		return queue.JobTypeGIF, true
	case "screenshot":
		return queue.JobTypeScreenshot, true
	default:
		return "", false
	}
}

// AssignOutputPaths sets the on-disk destination for a job.
//
// Parameters:
//   - job: Clip job to update.
//   - store: Blob storage used to derive output paths.
func AssignOutputPaths(job *queue.Job, store storage.Blob) {
	switch job.Type {
	case queue.JobTypeClip:
		job.OutputPath = store.ClipPath(job.ID)
	case queue.JobTypeGIF:
		job.OutputPath = store.GifPath(job.ID)
	case queue.JobTypeScreenshot:
		job.OutputPath = store.ScreenshotPath(job.ID)
	default:
	}
}

// ApplyDefaults fills empty job fields with standard values.
//
// Parameters:
//   - job: Clip job to update in place.
func ApplyDefaults(job *queue.Job) {
	if job.Type == "" {
		job.Type = queue.JobTypeClip
	}

	if job.Quality == "" {
		job.Quality = DefaultQuality
	}

	if job.MediaType == "" {
		job.MediaType = DefaultMediaType
	}
}
