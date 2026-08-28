// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMediaItemError(t *testing.T) {
	t.Parallel()

	_ = t.Context()

	assert.Equal(t, "bad duration", mediaItemError(nil, "bad duration"))
	assert.Equal(t, "bad duration", mediaItemError(errNoPlexServer, "bad duration"))
	assert.Equal(t, mediaLoadFailedMsg, mediaItemError(errNoPlexServer, ""))
	assert.Empty(t, mediaItemError(nil, ""))
}
