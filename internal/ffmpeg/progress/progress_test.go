// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package progress

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProgressWriter(t *testing.T) {
	t.Parallel()

	var last int

	writer := &Writer{
		duration: 10 * time.Second,
		on:       func(percent int) { last = percent },
		buf:      bytes.Buffer{},
	}

	_, err := writer.Write([]byte("frame=1 time=00:00:05.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Equal(t, 50, last)
}

func TestProgressWriterReportsHoursAndMinutes(t *testing.T) {
	t.Parallel()

	var last int

	writer := &Writer{
		duration: time.Hour,
		on:       func(percent int) { last = percent },
		buf:      bytes.Buffer{},
	}

	_, err := writer.Write([]byte("frame=1 time=00:30:00.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Equal(t, 50, last)
}

func TestProgressWriterSkipsUnparseableTimestamp(t *testing.T) {
	t.Parallel()

	var last int

	writer := &Writer{
		duration: 10 * time.Second,
		on:       func(percent int) { last = percent },
		buf:      bytes.Buffer{},
	}

	_, err := writer.Write([]byte("frame=1 time=99999999999:00:00.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Zero(t, last, "an overflowing timestamp must not publish a percent")
}

func TestProgressWriterCapsAtNinetyNine(t *testing.T) {
	t.Parallel()

	var last int

	writer := &Writer{
		duration: 10 * time.Second,
		on:       func(percent int) { last = percent },
		buf:      bytes.Buffer{},
	}

	_, err := writer.Write([]byte("frame=1 time=00:00:20.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Equal(t, maxReportedPercent, last)
}
