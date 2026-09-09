// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package queue

import (
	"time"
)

// JobType represents the type of job.
type JobType string

// JobStatus represents the status of a job.
type JobStatus string

// Job represents a media processing job.
type Job struct {
	ID            string
	Type          JobType
	Name          string
	MediaID       string
	MediaTitle    string
	MediaType     string
	InputPath     string
	OutputPath    string
	StartTime     float64
	Duration      float64
	Quality       string
	Width         int
	FPS           int
	AudioIndex    int
	CropBlackBars bool
	WebSafeColor  bool
	Status        JobStatus
	Progress      int
	Error         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const (
	// JobTypeClip represents a clip job.
	JobTypeClip JobType = "clip"

	// JobTypeGIF represents a GIF job.
	JobTypeGIF JobType = "gif"

	// JobTypeScreenshot represents a screenshot job.
	JobTypeScreenshot JobType = "screenshot"

	// JobStatusPending represents a pending job.
	JobStatusPending JobStatus = "pending"

	// JobStatusProcessing represents a processing job.
	JobStatusProcessing JobStatus = "processing"

	// JobStatusCompleted represents a completed job.
	JobStatusCompleted JobStatus = "completed"

	// JobStatusFailed represents a failed job.
	JobStatusFailed JobStatus = "failed"

	// JobStatusCancelled represents a job stopped by the user.
	JobStatusCancelled JobStatus = "canceled"
)
