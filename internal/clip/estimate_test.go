// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRemaining covers the estimate: the rate so far carried over the rest,
// and no estimate before the encode has run long enough or after it is done.
func TestRemaining(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		started time.Time
		percent int
		want    time.Duration
		ok      bool
	}{
		{
			name:    "a quarter done in a minute",
			started: now.Add(-time.Minute),
			percent: 25,
			want:    3 * time.Minute,
			ok:      true,
		},
		{
			name:    "half done",
			started: now.Add(-90 * time.Second),
			percent: 50,
			want:    90 * time.Second,
			ok:      true,
		},
		{name: "not started", started: time.Time{}, percent: 50},
		{name: "too little progress", started: now.Add(-time.Minute), percent: 2},
		{name: "too little time", started: now.Add(-4 * time.Second), percent: 50},
		{name: "done", started: now.Add(-time.Minute), percent: 100},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			left, ok := Remaining(test.started, now, test.percent)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, test.want, left)
		})
	}
}
