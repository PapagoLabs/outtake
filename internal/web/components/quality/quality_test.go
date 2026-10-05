// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package quality

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/web/utils"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func testOptions() []profile.ProfileOption {
	return []profile.ProfileOption{
		{ID: "low", Name: "Low"},
		{ID: "medium", Name: "Medium", IsDefault: true},
		{ID: "high", Name: "High"},
	}
}

func renderQualitySelect(t *testing.T, selected string) string {
	t.Helper()

	var buf strings.Builder

	err := QualitySelect("quality", testOptions(), selected).Render(t.Context(), &buf)
	require.NoError(t, err)

	return buf.String()
}

func TestQualitySelectMarksTheSubmittedProfile(t *testing.T) {
	t.Parallel()

	body := renderQualitySelect(t, "high")

	assert.Contains(t, body, `<select id="quality" name="quality" class="`+utils.ControlClass+`">`)
	assert.Contains(t, body, `<option value="low">Low</option>`)
	assert.Contains(t, body, `<option value="medium">Medium</option>`)
	assert.Contains(t, body, `<option value="high" selected>High</option>`)
	assert.Equal(t, 1, strings.Count(body, "selected"),
		"only the chosen profile is selected")
}

func TestQualitySelectFallsBackToTheDefaultProfile(t *testing.T) {
	t.Parallel()

	body := renderQualitySelect(t, "")

	assert.Contains(t, body, `<option value="medium" selected>Medium</option>`)
	assert.Equal(t, 1, strings.Count(body, "selected"),
		"an unsubmitted form opens on the default profile")
}

func TestQualitySelectMarksAnUnknownProfileAsChosen(t *testing.T) {
	t.Parallel()

	body := renderQualitySelect(t, "ultra")

	assert.NotContains(t, body, "selected",
		"a profile the page does not offer selects nothing")
}

func TestQualitySelectRendersNothingWithoutProfiles(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := QualitySelect("quality", nil, "").Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `<select id="quality" name="quality"`)
	assert.NotContains(t, body, "<option")
}

func TestAudioSelectMarksTheChosenTrack(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	tracks := []view.AudioTrackOption{
		{Index: 0, Label: "English"},
		{Index: 2, Label: "Commentary"},
	}

	err := AudioSelect("audioIndex", tracks, 2).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `<select id="audioIndex" name="audioIndex" class="`+utils.ControlClass+`">`)
	assert.Contains(t, body, `<option value="0">English</option>`)
	assert.Contains(t, body, `<option value="2" selected>Commentary</option>`)
	assert.Equal(t, 1, strings.Count(body, "selected"))
}

func TestAudioSelectKeepsAnUnlistedSelection(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	tracks := []view.AudioTrackOption{
		{Index: 0, Label: "English"},
		{Index: 2, Label: "Commentary"},
	}

	err := AudioSelect("audioIndex", tracks, 7).Render(t.Context(), &buf)
	require.NoError(t, err)

	assert.NotContains(t, buf.String(), "selected",
		"a track the form posted but the source no longer carries selects nothing")
}

func TestAudioSelectFallsBackToAHiddenInput(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := AudioSelect("audioIndex", nil, 3).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `<input type="hidden" name="audioIndex" value="3">`)
	assert.NotContains(t, body, "<select",
		"a source with no audio tracks offers nothing to choose from")
}
