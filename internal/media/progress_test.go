// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHMS(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 3661.5, parseHMS("1", "1", "1.5"), 0.001)
}

func TestProgressWriter(t *testing.T) {
	t.Parallel()

	var last int

	writer := &progressWriter{
		duration: 10,
		on:       func(percent int) { last = percent },
		buf:      bytes.Buffer{},
	}

	_, err := writer.Write([]byte("frame=1 time=00:00:05.00 bitrate=1\n"))
	require.NoError(t, err)
	assert.Equal(t, 50, last)
}
