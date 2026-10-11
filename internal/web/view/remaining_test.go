// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRemainingLabel covers how the time left reads on a progress bar: to
// the second in minutes and seconds under an hour, in hours and minutes past
// it, never zero, and the empty label before an estimate.
func TestRemainingLabel(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		elapsed time.Duration
		percent int
		want    string
	}{
		{name: "no estimate yet", elapsed: 2 * time.Second, percent: 50, want: ""},
		{name: "seconds", elapsed: 35 * time.Second, percent: 50, want: "35s left"},
		{name: "never zero", elapsed: 6 * time.Second, percent: 99, want: "1s left"},
		{name: "minutes and seconds", elapsed: 77 * time.Second, percent: 50, want: "1m 17s left"},
		{name: "whole minutes", elapsed: time.Minute, percent: 25, want: "3m left"},
		{name: "whole hours", elapsed: time.Hour, percent: 50, want: "1h left"},
		{name: "hours and minutes", elapsed: 20 * time.Minute, percent: 20, want: "1h 20m left"},
		{
			name:    "hours drop the seconds",
			elapsed: time.Hour + 30*time.Second,
			percent: 50,
			want:    "1h left",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, RemainingLabel(now.Add(-test.elapsed), now, test.percent))
		})
	}
}

// TestBarStyle covers a bar's width: a tenth of a percent, so a slow encode's
// bar moves between whole percents.
func TestBarStyle(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "width: 42.6%", BarStyle(42.63))
	assert.Equal(t, "width: 0.0%", BarStyle(0))
	assert.Equal(t, "width: 100.0%", BarStyle(100))
}
