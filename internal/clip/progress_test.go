// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBarPercent covers when the exact percent draws the bar.
//
// It draws the bar only while it refines the whole percent, never when it
// disagrees with it.
func TestBarPercent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		whole int
		exact float64
		want  float64
	}{
		{name: "refines the whole percent", whole: 42, exact: 42.6, want: 42.6},
		{name: "equals the whole percent", whole: 42, exact: 42, want: 42},
		{name: "never reported", whole: 42, exact: 0, want: 42},
		{name: "left from an earlier encode", whole: 0, exact: 99.4, want: 0},
		{name: "a whole percent ahead", whole: 42, exact: 43, want: 42},
		{name: "finished", whole: 100, exact: 99.9, want: 100},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, test.want, BarPercent(test.whole, test.exact), 1e-9)
		})
	}
}
