// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/web/api"
	"github.com/PapagoLabs/outtake/internal/clip/queue"
)

func TestApplyClipEditsPreservesOmittedWebSafeColor(t *testing.T) {
	t.Parallel()

	job := testClipJob("clip-1", queue.JobTypeClip)

	job.WebSafeColor = true

	applyClipEdits(job, api.ClipRequest{
		StartTime: 1,
		Duration:  5,
	})

	assert.True(t, job.WebSafeColor)

	off := false
	applyClipEdits(job, api.ClipRequest{
		StartTime:    1,
		Duration:     5,
		WebSafeColor: &off,
	})

	assert.False(t, job.WebSafeColor)
}

func TestDerefBool(t *testing.T) {
	t.Parallel()

	assert.False(t, derefBool(nil))
	assert.True(t, derefBool(new(true)))
	assert.False(t, derefBool(new(false)))
}
