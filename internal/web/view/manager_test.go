// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestViewerManagesReadsTheRecordedFlag covers the flag the layout reads,
// which is false when nothing recorded it.
func TestViewerManagesReadsTheRecordedFlag(t *testing.T) {
	t.Parallel()

	assert.True(t, ViewerManages(ContextWithManager(t.Context(), true)))
	assert.False(t, ViewerManages(ContextWithManager(t.Context(), false)))
	assert.False(t, ViewerManages(t.Context()))
}
