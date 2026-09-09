// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package html

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClipProfileFromFields(t *testing.T) {
	t.Parallel()

	const slowPreset = "slow"

	profile, err := clipProfileFromFields("id-1", " Archive ", "18", slowPreset, "320", "3840")
	require.NoError(t, err)
	assert.Equal(t, "id-1", profile.ID)
	assert.Equal(t, "Archive", profile.Name)
	assert.Equal(t, 18, profile.CRF)
	assert.Equal(t, "slow", profile.Preset)
	assert.Equal(t, 320, profile.AudioKbps)
	assert.Equal(t, 3840, profile.MaxWidth)

	_, err = clipProfileFromFields("id-1", "  ", "18", slowPreset, "320", "3840")
	require.ErrorIs(t, err, errProfileName)

	_, err = clipProfileFromFields(
		"id-1",
		strings.Repeat("a", maxProfileNameLen+1),
		"18",
		slowPreset,
		"320",
		"3840",
	)
	require.ErrorIs(t, err, errProfileNameLength)

	_, err = clipProfileFromFields("id-1", "Archive", "99", slowPreset, "320", "3840")
	require.ErrorIs(t, err, errProfileCRF)

	_, err = clipProfileFromFields("id-1", "Archive", "18", "turbo", "320", "3840")
	require.ErrorIs(t, err, errProfilePreset)

	_, err = clipProfileFromFields("id-1", "Archive", "18", slowPreset, "12", "3840")
	require.ErrorIs(t, err, errProfileAudio)

	_, err = clipProfileFromFields("id-1", "Archive", "18", slowPreset, "320", "1000")
	require.ErrorIs(t, err, errProfileWidth)
}

func TestBuiltinProfileOptions(t *testing.T) {
	t.Parallel()

	options := builtinProfileOptions()
	require.Len(t, options, 3)
	assert.Equal(t, "medium", options[1].ID)
	assert.True(t, options[1].IsDefault)
}
