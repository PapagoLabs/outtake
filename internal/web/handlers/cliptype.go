// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"github.com/PapagoLabs/outtake/internal/queue"
	"github.com/PapagoLabs/outtake/internal/storage"
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

// assignOutputPaths sets the on-disk destination for a job.
func assignOutputPaths(job *queue.Job, store storage.Blob) {
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

// applyDefaults fills empty job fields with standard values.
func applyDefaults(job *queue.Job) {
	if job.Type == "" {
		job.Type = queue.JobTypeClip
	}

	if job.Quality == "" {
		job.Quality = defaultQuality
	}

	if job.MediaType == "" {
		job.MediaType = defaultMediaType
	}
}
