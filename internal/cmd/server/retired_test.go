// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSetRetiredSettingsFindsTheHDRSwitches covers the startup warning: the
// global HDR switches from earlier releases are reported when set, even to
// false, and nothing is reported for an environment without them.
func TestSetRetiredSettingsFindsTheHDRSwitches(t *testing.T) {
	t.Parallel()

	env := map[string]string{"OUTTAKE_WEB_SAFE_COLOR": "false", "OUTTAKE_LOG_LEVEL": "info"}
	lookup := func(name string) (string, bool) {
		value, ok := env[name]

		return value, ok
	}

	set := setRetiredSettings(lookup)
	if assert.Len(t, set, 1) {
		assert.Equal(t, "OUTTAKE_WEB_SAFE_COLOR", set[0].name)
		assert.Equal(t, profileKeepsHDR, set[0].replacement)
	}

	assert.Empty(t, setRetiredSettings(func(string) (string, bool) { return "", false }))
}
