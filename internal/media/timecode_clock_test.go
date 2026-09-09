// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubClock struct {
	parse time.Duration
}

// Format returns the format.
//
// Parameters:
//   - duration: Argument for Format.
//
// Returns:
//   - value: The format.
func (stubClock) Format(time.Duration) string { return "stub" }

// Parse returns the parse.
//
// Parameters:
//   - string: Argument for Parse.
//
// Returns:
//   - dur: The parse.
//   - err: Non-nil when parsing fails.
func (clock stubClock) Parse(string) (time.Duration, error) {
	return clock.parse, nil
}

//nolint:paralleltest // DefaultClock is process-wide.
func TestParseUsesDefaultClock(t *testing.T) {
	original := DefaultClock

	t.Cleanup(func() { DefaultClock = original })

	DefaultClock = stubClock{parse: 42 * time.Second}

	got, err := Parse("anything")
	require.NoError(t, err)
	assert.Equal(t, 42*time.Second, got.Duration())
}

func TestParseNegativeDuration(t *testing.T) {
	t.Parallel()

	got, err := Parse("-1s")
	require.NoError(t, err)
	assert.Equal(t, -time.Second, got.Duration())
}
