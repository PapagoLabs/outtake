// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestExecFFmpeg_DeadlineScalesWithTheClip covers the default limit: at least
// half an hour, and twenty times a long clip's length.
func TestExecFFmpeg_DeadlineScalesWithTheClip(t *testing.T) {
	t.Parallel()

	execFFmpeg := NewExecFFmpeg("sleep", "sleep")

	assert.Equal(t, 30*time.Minute, execFFmpeg.deadline(0), "an unknown length gets the floor")
	assert.Equal(t, 30*time.Minute, execFFmpeg.deadline(time.Minute), "a short clip gets the floor")
	assert.Equal(t, 200*time.Minute, execFFmpeg.deadline(10*time.Minute),
		"a long 4K encode is not killed at the floor")
}

// TestExecFFmpeg_WithTimeoutFixesTheDeadline covers ffmpeg-timeout-sec: a
// positive value replaces the scaled limit, and zero or less keeps it.
func TestExecFFmpeg_WithTimeoutFixesTheDeadline(t *testing.T) {
	t.Parallel()

	base := NewExecFFmpeg("sleep", "sleep")

	fixed := base.WithTimeout(5 * time.Minute)
	assert.Equal(t, 5*time.Minute, fixed.deadline(10*time.Minute))
	assert.Equal(t, 30*time.Minute, base.deadline(0), "the original executor is unchanged")

	assert.Equal(t, 200*time.Minute, base.WithTimeout(-time.Second).deadline(10*time.Minute))
}

// TestExecFFmpeg_EncodeDeadlineGivesHEVCMoreRoom covers the limit for each
// encoder: an HEVC encode may take sixty times its clip's length, any other
// encode twenty times, both above the half-hour floor, and a configured
// ffmpeg-timeout-sec applies to every encoder alike.
func TestExecFFmpeg_EncodeDeadlineGivesHEVCMoreRoom(t *testing.T) {
	t.Parallel()

	execFFmpeg := NewExecFFmpeg("sleep", "sleep")

	assert.Equal(t, 600*time.Minute, execFFmpeg.encodeDeadline(10*time.Minute, videoCodecHEVC))
	assert.Equal(t, 200*time.Minute, execFFmpeg.encodeDeadline(10*time.Minute, videoCodecH264))
	assert.Equal(t, 30*time.Minute, execFFmpeg.encodeDeadline(time.Second, videoCodecHEVC),
		"a short HEVC clip gets the floor")

	fixed := execFFmpeg.WithTimeout(5 * time.Minute)
	assert.Equal(t, 5*time.Minute, fixed.encodeDeadline(10*time.Minute, videoCodecHEVC))
	assert.Equal(t, 5*time.Minute, fixed.encodeDeadline(10*time.Minute, videoCodecH264))
}
