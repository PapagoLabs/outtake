// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
)

// NewClipItem maps a job onto a card the clips list and a clip update both show.
//
// Parameters:
//   - job: Render the card is built from.
//   - profiles: Profiles the card offers as a quality choice.
//   - limit: Longest clip the installation accepts.
//   - fileExists: Whether the job's output is still on disk.
//
// Returns:
//   - item: Page model for the clip card.
func NewClipItem(
	job *clip.Job,
	options []profile.ProfileOption,
	limit time.Duration,
	fileExists bool,
) ClipItem {
	return ClipItem{
		ID:            job.ID,
		Name:          job.Name,
		MediaID:       job.MediaID,
		MediaTitle:    job.MediaTitle,
		ClipType:      job.Type,
		Status:        job.Status,
		Progress:      job.Progress,
		CreatedAt:     FormatClipCreated(job.CreatedAt),
		Error:         job.Error,
		StartTime:     job.StartTime,
		Duration:      job.Duration,
		Quality:       job.Quality,
		ProfileName:   profile.ProfileName(job.Quality, options),
		Profiles:      options,
		FileExists:    fileExists,
		AudioIndex:    job.AudioIndex,
		AudioTracks:   nil,
		CropBlackBars: job.CropBlackBars,
		WebSafeColor:  job.WebSafeColor,
		PreserveHDR:   job.PreserveHDR,
		Width:         job.Width,
		FPS:           job.FPS,
		MaxDur:        limit,
	}
}

// NewClipItems maps jobs onto the cards the clips list and a media item page
// show, which resolve the profiles and the output checks the same way for every
// row.
//
// Parameters:
//   - jobs: Renders to show.
//   - options: Profiles every card offers as a quality choice.
//   - limit: Longest clip the installation accepts.
//   - fileExists: Whether a job's output is still on disk.
//
// Returns:
//   - items: One page model per clip, in the order the clips were given.
func NewClipItems(
	jobs []*clip.Job,
	options []profile.ProfileOption,
	limit time.Duration,
	fileExists func(string) bool,
) []ClipItem {
	items := make([]ClipItem, 0, len(jobs))

	for _, job := range jobs {
		items = append(items, NewClipItem(job, options, limit, fileExists(job.OutputPath)))
	}

	return items
}
