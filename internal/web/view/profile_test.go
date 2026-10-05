// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip/profile"
)

// storedOnlyFields are the stored profile fields the settings page has no use
// for. Every other field has to reach the page model under the same name.
var storedOnlyFields = map[string]bool{"CreatedAt": true, "UpdatedAt": true}

func TestClipProfileItemsMapsEveryField(t *testing.T) {
	t.Parallel()

	items := ClipProfileItems([]profile.Profile{
		{
			ID:        "profile-1",
			Name:      "Podcast",
			CRF:       28,
			Preset:    "faster",
			AudioKbps: 192,
			MaxWidth:  1440,
			IsDefault: true,
		},
		{
			ID:        "profile-2",
			Name:      "Archive",
			CRF:       18,
			Preset:    "slow",
			AudioKbps: 256,
			MaxWidth:  3840,
		},
	})

	require.Len(t, items, 2)
	assert.Equal(t, ClipProfileItem{
		ID:        "profile-1",
		Name:      "Podcast",
		CRF:       28,
		Preset:    "faster",
		AudioKbps: 192,
		MaxWidth:  1440,
		IsDefault: true,
	}, items[0])
	assert.Equal(t, ClipProfileItem{
		ID:        "profile-2",
		Name:      "Archive",
		CRF:       18,
		Preset:    "slow",
		AudioKbps: 256,
		MaxWidth:  3840,
	}, items[1],
		"a profile that is not the default still reaches the page with its own values")
}

func TestClipProfileItemFieldsMirrorTheStoredProfile(t *testing.T) {
	t.Parallel()

	stored := reflect.TypeFor[profile.Profile]()
	pageFields := exportedFieldNames(reflect.TypeFor[ClipProfileItem]())

	require.Len(t, pageFields, stored.NumField()-len(storedOnlyFields),
		"ClipProfileItem gained or lost a field, so a profile row no longer round-trips")

	for field := range stored.Fields() {
		if storedOnlyFields[field.Name] {
			continue
		}

		assert.Contains(t, pageFields, field.Name,
			"profile.Profile.%s has no matching ClipProfileItem field and would render blank",
			field.Name)
	}
}

func TestClipProfileItemsAreEmptyForNoProfiles(t *testing.T) {
	t.Parallel()

	assert.Empty(t, ClipProfileItems(nil))
}

// exportedFieldNames lists the exported field names of a struct type.
//
// Parameters:
//   - structType: Struct type to inspect.
//
// Returns:
//   - names: Every exported field name, in declaration order.
func exportedFieldNames(structType reflect.Type) []string {
	names := make([]string, 0, structType.NumField())

	for field := range structType.Fields() {
		if !field.IsExported() {
			continue
		}

		names = append(names, field.Name)
	}

	return names
}
