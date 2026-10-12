// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestLastKeyframe covers reading ffprobe's packet list for a keyframe.
//
// The last keyframe in the range wins, a non-keyframe or an unreadable time
// is skipped, and the time is rounded up to the millisecond, so a mark set
// from it seeks onto the same keyframe again.
func TestLastKeyframe(t *testing.T) {
	t.Parallel()

	packets := "2.900000,___\n3.000000,K__\nN/A,K__\n5.958000,___\n6.336667,K_\n9.000000,K__\n"

	keyframe, found := lastKeyframe(packets, 2*time.Second, 7300*time.Millisecond)
	assert.True(t, found)
	assert.Equal(t, 6337*time.Millisecond, keyframe, "rounded up, so the mark stays on it")

	keyframe, found = lastKeyframe(packets, 2*time.Second, 6337*time.Millisecond)
	assert.True(t, found)
	assert.Equal(t, 6337*time.Millisecond, keyframe, "the recorded start finds the same keyframe")

	_, found = lastKeyframe(packets, 6400*time.Millisecond, 8*time.Second)
	assert.False(t, found, "no keyframe in the range")

	_, found = lastKeyframe("", 0, time.Minute)
	assert.False(t, found)
}
