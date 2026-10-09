// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"path/filepath"
	"strings"
)

// Stage is which encode of a render is running.
type Stage string

const (
	// StageClip is the encode of the clip's own file, the only one most
	// renders have.
	StageClip Stage = ""
	// StageSDR is the encode of an HDR clip's SDR version, which follows the
	// clip's own file.
	StageSDR Stage = "sdr"
)

// sdrSuffix ends the name of a clip's SDR version, beside the clip file.
const sdrSuffix = ".sdr.mp4"

// SDRPathFor names the SDR version that belongs beside a video clip's file.
//
// Parameters:
//   - output: A video clip's output path.
//
// Returns:
//   - path: The SDR version's path, empty when output is not an MP4 file.
func SDRPathFor(output string) string {
	if !strings.EqualFold(filepath.Ext(output), ".mp4") {
		return ""
	}

	return strings.TrimSuffix(output, filepath.Ext(output)) + sdrSuffix
}

// SDRPath names the job's SDR version. Only a video clip has one.
//
// Returns:
//   - path: The SDR version's path, empty for a GIF, a screenshot, or a job
//     with no output path.
func (job *Job) SDRPath() string {
	if job.Type != TypeClip {
		return ""
	}

	return SDRPathFor(job.OutputPath)
}

// MayHaveSDRVersion reports whether the job may have an SDR version: a video
// clip that keeps HDR. Whether its source is HDR is decided when it renders.
//
// Returns:
//   - may: True when an SDR version may be stored beside the clip.
func (job *Job) MayHaveSDRVersion() bool {
	return job.Type == TypeClip && job.PreserveHDR && job.SDRPath() != ""
}

// SDRPaths lists the SDR version path of each job that may have one.
//
// Parameters:
//   - jobs: The jobs to read.
//
// Returns:
//   - paths: The SDR version paths, in job order, skipping jobs with none.
func SDRPaths(jobs []*Job) []string {
	paths := make([]string, 0, len(jobs))

	for _, job := range jobs {
		if job.MayHaveSDRVersion() {
			paths = append(paths, job.SDRPath())
		}
	}

	return paths
}
