// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

// Job is one render of a clip: the record, where the files are, and how far
// the render has got.
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

	// Error is the failure a stopped render reports. Empty when it has not failed.
	Error string
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
