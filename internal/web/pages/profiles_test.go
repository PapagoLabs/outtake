// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pages

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

func testWidths() []profile.OutputWidth {
	return []profile.OutputWidth{
		{Width: 720, Label: "720p"},
		{Width: 1080, Label: "1080p"},
		{Width: 1920, Label: "1080p"},
	}
}

func testPresets() []string {
	return []string{"fast", "medium", "slow"}
}

func renderClipProfiles(t *testing.T, props ClipProfilesProps) string {
	t.Helper()

	var buf strings.Builder

	err := ClipProfiles(props).Render(
		identity.ContextWithCSRFToken(t.Context(), "profile-csrf"), &buf,
	)
	require.NoError(t, err)

	return buf.String()
}

func TestClipProfilesRendersTheNewProfileForm(t *testing.T) {
	t.Parallel()

	body := renderClipProfiles(t, ClipProfilesProps{
		Presets: testPresets(),
		Widths:  testWidths(),
	})

	assert.Contains(t, body, `<form action="/settings/profiles" method="POST"`)
	assert.Contains(t, body, "New Profile")
	assert.Contains(t, body, `id="new-name"`)
	assert.Contains(t, body, `name="name"`)
	assert.Contains(
		t,
		body,
		`value="20"`,
		"the new profile form opens on the 1080p settings, CRF 20",
	)
	assert.Contains(t, body, `value="192"`, "the new profile form opens on 192 kbps")
	assert.Contains(t, body, `<option value="medium" selected>medium</option>`)
	assert.Contains(t, body, `<option value="1920" selected>1080p</option>`)
	assert.Contains(t, body, "Set as default")
	assert.Contains(t, body, "Add Profile")
	assert.Contains(t, body, `<input type="hidden" name="_csrf" value="profile-csrf">`)
}

func TestClipProfilesRendersAStoredProfile(t *testing.T) {
	t.Parallel()

	body := renderClipProfiles(t, ClipProfilesProps{
		Profiles: []view.ClipProfileItem{{
			ID:        "profile-archive",
			Name:      "Archive",
			CRF:       20,
			Preset:    "slow",
			AudioKbps: 256,
			MaxWidth:  1080,
			IsDefault: true,
		}},
		Presets: testPresets(),
		Widths:  testWidths(),
	})

	assert.Contains(t, body, `action="/settings/profiles/profile-archive"`)
	assert.Contains(t, body, `id="profile-profile-archive-name"`)
	assert.Contains(t, body, `value="Archive"`)
	assert.Contains(t, body, `value="20"`)
	assert.Contains(t, body, `value="256"`)
	assert.Contains(t, body, `<option value="slow" selected>slow</option>`)
	assert.Contains(t, body, `<option value="1080" selected>1080p</option>`)
	assert.Contains(t, body, `min="0"`, "the CRF field is bounded by the codec")
	assert.Contains(t, body, `max="51"`)
	assert.Contains(t, body, `min="64"`)
	assert.Contains(t, body, `max="640"`)
	assert.Contains(t, body, "Save")
}

func TestClipProfilesMarksTheDefaultProfile(t *testing.T) {
	t.Parallel()

	body := renderClipProfiles(t, ClipProfilesProps{
		Profiles: []view.ClipProfileItem{{
			ID:        "profile-archive",
			Name:      "Archive",
			IsDefault: true,
		}},
		Presets: testPresets(),
		Widths:  testWidths(),
	})

	assert.Contains(t, body, ">Default</span>")
	assert.NotContains(t, body, "Make Default")
	assert.NotContains(t, body, `id="default-profile-archive"`)
	assert.NotContains(t, body, "Delete",
		"the only profile cannot be deleted, since one must remain")
}

func TestClipProfilesOffersMakeDefaultOnANonDefaultProfile(t *testing.T) {
	t.Parallel()

	body := renderClipProfiles(t, ClipProfilesProps{
		Profiles: []view.ClipProfileItem{
			{ID: "profile-archive", Name: "Archive"},
			{ID: "profile-other", Name: "Other"},
		},
		Presets: testPresets(),
		Widths:  testWidths(),
	})

	assert.NotContains(t, body, ">Default</span>")
	assert.Contains(t, body, "Make Default")
	assert.Contains(t, body, `form="default-profile-archive"`)
	assert.Contains(t, body, `id="default-profile-archive"`)
	assert.Contains(t, body, `action="/settings/profiles/profile-archive/default"`)
	assert.Contains(t, body, `id="default-profile-other"`)
	assert.Contains(t, body, "Delete",
		"with more than one profile the last one can be removed")
	assert.Contains(t, body, `form="delete-profile-archive"`)
	assert.Contains(t, body, `data-confirm="Delete this profile? This can't be undone."`)
}

func TestClipProfilesShowsTheSaveError(t *testing.T) {
	t.Parallel()

	body := renderClipProfiles(t, ClipProfilesProps{
		Error:   "That profile name is already taken.",
		Presets: testPresets(),
		Widths:  testWidths(),
	})

	assert.Contains(t, body, "That profile name is already taken.")
	assert.Contains(t, body, `id="flash"`)
}

func TestClipProfilesOmitsTheSaveErrorBannerWhenThereIsNone(t *testing.T) {
	t.Parallel()

	body := renderClipProfiles(t, ClipProfilesProps{
		Presets: testPresets(),
		Widths:  testWidths(),
	})

	assert.NotContains(t, body, "js-flash")
	assert.Contains(t, body, "New Profile",
		"the new profile form is offered even with nothing stored")
}

func TestClipProfilesOmitsUnselectedProfileOptions(t *testing.T) {
	t.Parallel()

	body := renderClipProfiles(t, ClipProfilesProps{
		Presets: testPresets(),
		Widths:  testWidths(),
	})

	assert.Contains(t, body, `<option value="fast">fast</option>`)
	assert.Contains(t, body, `<option value="slow">slow</option>`)
	assert.Contains(t, body, `<option value="720">720p</option>`)
	assert.Contains(t, body, `<option value="1080">1080p</option>`)
}

func TestClipProfilesRendersNothingToEditWithoutProfiles(t *testing.T) {
	t.Parallel()

	body := renderClipProfiles(t, ClipProfilesProps{
		Presets: testPresets(),
		Widths:  testWidths(),
	})

	assert.NotContains(t, body, "/settings/profiles/")
	assert.NotContains(t, body, ">Save</button>",
		"there is no stored profile to save")
	assert.Equal(t, 1, strings.Count(body, `action="/settings/profiles" method="POST"`),
		"only the new profile form is offered")
}
