// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give *bool
		want bool
	}{
		{name: "an absent flag is off", give: nil, want: false},
		{name: "an explicit on is on", give: new(true), want: true},
		{name: "an explicit off is off", give: new(false), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, Flag(test.give))
		})
	}
}

func TestFlagOrDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		give     *bool
		fallback bool
		want     bool
	}{
		{
			name:     "an absent flag takes the configured default",
			give:     nil,
			fallback: true,
			want:     true,
		},
		{
			name:     "an absent flag with no default stays off",
			give:     nil,
			fallback: false,
			want:     false,
		},
		{
			name:     "an explicit on is honored against a false default",
			give:     new(true),
			fallback: false,
			want:     true,
		},
		{
			name:     "an explicit off is honored against a true default",
			give:     new(false),
			fallback: true,
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, FlagOrDefault(test.give, test.fallback))
		})
	}
}
