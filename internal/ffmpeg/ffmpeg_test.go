// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultFFmpegTimeout(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 30*60*1000000000, int(DefaultFFmpegTimeout()))
}
