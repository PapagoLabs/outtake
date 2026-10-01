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
	// PreserveHDR keeps an HDR source as it is instead of tone mapping it.
	// It is per clip, like WebSafeColor, because the two interact and a user
	// decides both at the point they make the clip.
	PreserveHDR bool
	Status      JobStatus
	Progress    int
	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
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

	// JobStatusCancelled represents a job stopped by the user. It doubles as the
	// Error text a stopped job carries, so the two never read as different
	// outcomes.
	JobStatusCancelled JobStatus = "canceled"
)

// clone returns an independent copy of the job.
//
// The copy is a value copy, so it is only independent while every field is
// scalar. Adding a pointer, slice or map field would leave the copy sharing
// whatever that field points at, which is the whole thing this exists to
// prevent, and a field added quietly would not fail any test — it would just
// reintroduce the shared state. A reference-typed field needs its own deep copy
// here.
//
// Returns:
//   - copy: A job that shares nothing with the original.
func (job *Job) clone() *Job {
	if job == nil {
		return nil
	}

	copied := *job

	return &copied
}
