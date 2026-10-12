// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"time"
)

// Job is one render of a clip.
//
// It holds the record, where the files are, and how far the render has got.
type Job struct {
	Clip

	// InputPath is the source file the render reads.
	InputPath string

	// OutputPath is the file the render writes. Empty until a path is assigned.
	OutputPath string

	// Status is how far this render has got.
	Status Status

	// Progress is the render's percent complete, from 0 to 100.
	Progress int

	// ExactProgress is the percent the running encode last reported, not
	// rounded, which draws its progress bar through [BarPercent]. Like Stage,
	// it is only known while the queue holds the job, and is never stored.
	ExactProgress float64

	// Error is the plain message a failed or canceled render reports. Empty
	// when it has not failed.
	Error string

	// ErrorDetails is the technical report behind Error, for an issue. Empty
	// when Error says all there is.
	ErrorDetails string

	// Stage is which encode of a running render Progress counts. It is only
	// known while the queue holds the job, and is never stored.
	Stage Stage

	// StageStarted is when the encode Stage names began, which the time left
	// is estimated from. Like Stage, it is only known while the queue holds
	// the job.
	StageStarted time.Time

	// OutputFormat is what the clip's own file holds, read when it was
	// published. It is zero for a GIF, a screenshot, or a file never read.
	OutputFormat Format

	// SDRFormat is what the clip's SDR version holds, zero when it has none.
	SDRFormat Format

	// OutputBytes is how many bytes the clip's files take, its own and its
	// SDR version, recorded when a render finishes. It counts toward its
	// owner's storage limit.
	OutputBytes int64
}

// BarPercent returns how far the job's progress bar is drawn.
//
// See [BarPercent] for when the exact percent is used.
//
// Returns:
//   - percent: How far to draw the bar, from 0 to 100.
func (job *Job) BarPercent() float64 {
	return BarPercent(job.Progress, job.ExactProgress)
}

// Clone returns an independent copy of the render.
//
// Returns:
//   - copy: A job that shares nothing with the original.
func (job *Job) Clone() *Job {
	if job == nil {
		return nil
	}

	copied := *job

	return &copied
}
