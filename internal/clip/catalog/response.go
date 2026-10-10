// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"github.com/PapagoLabs/outtake/internal/clip"
)

// ClipResponse maps a job onto the public payload the API returns, withholding
// the internal input and output paths.
//
// Parameters:
//   - job: Render to map.
//
// Returns:
//   - response: The payload.
func ClipResponse(job *clip.Job) clip.Response {
	return clip.Response{
		ID:            job.ID,
		Name:          job.Name,
		MediaID:       job.MediaID,
		MediaTitle:    job.MediaTitle,
		MediaType:     job.MediaType,
		ClipType:      job.Type,
		Status:        job.Status,
		Progress:      job.Progress,
		InputPath:     "",
		OutputPath:    "",
		Error:         job.Error,
		ErrorDetails:  job.ErrorDetails,
		CreatedAt:     job.CreatedAt,
		UpdatedAt:     job.UpdatedAt,
		AudioIndex:    job.AudioIndex,
		CropBlackBars: job.CropBlackBars,
		PreserveHDR:   job.PreserveHDR,
	}
}

// ClipResponses maps every job onto the public payload the API returns.
//
// Parameters:
//   - jobs: Renders to map.
//
// Returns:
//   - responses: One payload per job, in the order the jobs were given.
func ClipResponses(jobs []*clip.Job) []clip.Response {
	responses := make([]clip.Response, 0, len(jobs))

	for _, job := range jobs {
		responses = append(responses, ClipResponse(job))
	}

	return responses
}
