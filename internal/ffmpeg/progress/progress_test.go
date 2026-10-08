// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package progress

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProgressWriter(t *testing.T) {
	t.Parallel()

	var last int

	writer := NewWriter(10*time.Second, func(percent int) { last = percent })

	_, err := writer.Write([]byte("frame=1 time=00:00:05.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Equal(t, 50, last)
}

func TestProgressWriterReportsHoursAndMinutes(t *testing.T) {
	t.Parallel()

	var last int

	writer := NewWriter(time.Hour, func(percent int) { last = percent })

	_, err := writer.Write([]byte("frame=1 time=00:30:00.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Equal(t, 50, last)
}

func TestProgressWriterSkipsUnparseableTimestamp(t *testing.T) {
	t.Parallel()

	var last int

	writer := NewWriter(10*time.Second, func(percent int) { last = percent })

	_, err := writer.Write([]byte("frame=1 time=99999999999:00:00.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Zero(t, last, "an overflowing timestamp must not publish a percent")
}

func TestProgressWriterCapsAtNinetyNine(t *testing.T) {
	t.Parallel()

	var last int

	writer := NewWriter(10*time.Second, func(percent int) { last = percent })

	_, err := writer.Write([]byte("frame=1 time=00:00:20.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Equal(t, maxReportedPercent, last)
}

// TestProgressWriterReadsATimeSplitAcrossWrites covers a progress line that
// ffmpeg's pipe delivers in two pieces.
func TestProgressWriterReadsATimeSplitAcrossWrites(t *testing.T) {
	t.Parallel()

	var reported []int

	writer := NewWriter(10*time.Second, func(percent int) { reported = append(reported, percent) })

	_, err := writer.Write([]byte("frame=1 ti"))
	require.NoError(t, err)
	assert.Empty(t, reported, "half a line carries no time")

	_, err = writer.Write([]byte("me=00:00:03.00 bitrate=1\r"))
	require.NoError(t, err)
	assert.Equal(t, []int{30}, reported)
}

// TestProgressWriterReportsOnlyAChangedPercent covers repeated progress lines
// at the same percent, which ffmpeg emits several times a second.
func TestProgressWriterReportsOnlyAChangedPercent(t *testing.T) {
	t.Parallel()

	var reported []int

	writer := NewWriter(100*time.Second, func(percent int) { reported = append(reported, percent) })

	for _, line := range []string{
		"time=00:00:01.10\r", "time=00:00:01.20\r", "time=00:00:02.00\r", "time=00:00:02.50\r",
	} {
		_, err := writer.Write([]byte(line))
		require.NoError(t, err)
	}

	assert.Equal(t, []int{1, 2}, reported)
}

// TestProgressWriterKeepsOnlyTheTail covers memory: a long encode's stderr is
// counted in full but only its tail is kept, and a long run without a line
// break is not carried whole.
func TestProgressWriterKeepsOnlyTheTail(t *testing.T) {
	t.Parallel()

	writer := NewWriter(time.Hour, func(int) {})

	chunk := strings.Repeat("x", 1000) + "\n"
	for range 200 {
		_, err := writer.Write([]byte(chunk))
		require.NoError(t, err)
	}

	_, err := writer.Write([]byte("last line\n"))
	require.NoError(t, err)

	assert.Equal(t, 200*len(chunk)+len("last line\n"), writer.Len(), "every byte is counted")
	assert.Len(t, writer.String(), TailSize, "only the tail is kept")
	assert.True(
		t,
		strings.HasSuffix(writer.String(), "last line\n"),
		"and it ends with the newest output",
	)

	_, err = writer.Write([]byte(strings.Repeat("y", 10*maxPartial)))
	require.NoError(t, err)
	assert.Len(
		t,
		writer.partial,
		maxPartial,
		"an unbroken run is carried only up to its last bytes",
	)
}

// TestProgressWriterKeepsAWholeOversizedWrite covers a single write larger than
// the tail.
func TestProgressWriterKeepsAWholeOversizedWrite(t *testing.T) {
	t.Parallel()

	writer := NewWriter(time.Hour, nil)

	big := strings.Repeat("a", TailSize) + "end"

	_, err := writer.Write([]byte(big))
	require.NoError(t, err)

	assert.Len(t, writer.String(), TailSize)
	assert.True(t, strings.HasSuffix(writer.String(), "end"))
}
