// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"strconv"
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
		FileVersion:   strconv.FormatInt(job.UpdatedAt.UnixMilli(), fileVersionBase),
		Error:         job.Error,
		ErrorDetails:  job.ErrorDetails,
		StartTime:     job.StartTime,
		Duration:      job.Duration,
		Quality:       job.Quality,
		ProfileName:   profile.ProfileName(job.Quality, options),
		Profiles:      options,
		FileExists:    fileExists,
		AudioIndex:    job.AudioIndex,
		AudioTracks:   nil,
		CropBlackBars: job.CropBlackBars,
		PreserveHDR:   job.PreserveHDR,
		Stage:         job.Stage,
		OutputFormat:  job.OutputFormat,
		SDRFormat:     job.SDRFormat,
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
//   - fileExists: Whether a file is still stored, asked of each job's output
//     and of its SDR version when it may have one.
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
		item := NewClipItem(job, options, limit, fileExists(job.OutputPath))

		item.SDRExists = job.MayHaveSDRVersion() && fileExists(job.SDRPath())

		items = append(items, item)
	}

	return items
}
