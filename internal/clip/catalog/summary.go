// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package catalog

import (
	"github.com/PapagoLabs/outtake/internal/clip"
)

// Summary counts jobs by render status.
type Summary struct {
	// Total is every job known to the queue or the database.
	Total int
	// Pending counts jobs that are queued or still rendering.
	Pending int
	// Completed counts jobs that rendered successfully.
	Completed int
	// Failed counts jobs whose render failed.
	Failed int
}

// Summarize counts jobs by render status.
//
// Parameters:
//   - jobs: Renders currently known to the queue or the database.
//
// Returns:
//   - summary: Counters for each pending and terminal status.
func Summarize(jobs []*clip.Job) Summary {
	summary := Summary{Total: len(jobs)}

	for _, job := range jobs {
		switch job.Status {
		case clip.StatusPending, clip.StatusProcessing:
			summary.Pending++
		case clip.StatusCompleted:
			summary.Completed++
		case clip.StatusFailed:
			summary.Failed++
		default:
		}
	}

	return summary
}
