// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/store/database"
)

func TestProfileFromFields(t *testing.T) {
	t.Parallel()

	profile, err := ProfileFromFields("id-1", archiveFields())
	require.NoError(t, err)
	assert.Equal(t, "id-1", profile.ID)
	assert.Equal(t, "Archive", profile.Name)
	assert.Equal(t, 18, profile.CRF)
	assert.Equal(t, "slow", profile.Preset)
	assert.Equal(t, 320, profile.AudioKbps)
	assert.Equal(t, 3840, profile.MaxWidth)
	assert.False(t, profile.CreatedAt.IsZero())

	tests := []struct {
		name    string
		give    ProfileFields
		wantErr error
	}{
		{
			name:    "a missing name",
			give:    fieldsWith(t, func(f *ProfileFields) { f.Name = "  " }),
			wantErr: ErrProfileName,
		},
		{
			name: "an over-long name",
			give: fieldsWith(t, func(f *ProfileFields) {
				f.Name = strings.Repeat("a", MaxProfileNameLen+1)
			}),
			wantErr: ErrProfileNameLength,
		},
		{
			name:    "a CRF outside the range",
			give:    fieldsWith(t, func(f *ProfileFields) { f.CRF = "99" }),
			wantErr: ErrProfileCRF,
		},
		{
			name:    "an unknown encoder preset",
			give:    fieldsWith(t, func(f *ProfileFields) { f.Preset = "turbo" }),
			wantErr: ErrProfilePreset,
		},
		{
			name:    "an audio bitrate out of range",
			give:    fieldsWith(t, func(f *ProfileFields) { f.AudioKbps = "12" }),
			wantErr: ErrProfileAudio,
		},
		{
			name:    "a resolution that is not an export size",
			give:    fieldsWith(t, func(f *ProfileFields) { f.MaxWidth = "1000" }),
			wantErr: ErrProfileWidth,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := ProfileFromFields("id-1", test.give)
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestProfileFromFieldsCarriesTheRequestedDefault(t *testing.T) {
	t.Parallel()

	fields := fieldsWith(t, func(f *ProfileFields) { f.IsDefault = true })

	profile, err := ProfileFromFields("id-1", fields)
	require.NoError(t, err)

	assert.True(t, profile.IsDefault)
}

func TestProfileFromFieldsCarriesKeepHDR(t *testing.T) {
	t.Parallel()

	for _, keep := range []bool{true, false} {
		fields := fieldsWith(t, func(f *ProfileFields) { f.KeepHDR = keep })

		profile, err := ProfileFromFields("id-1", fields)
		require.NoError(t, err)

		assert.Equal(t, keep, profile.KeepHDR)
		assert.Equal(t, keep, profile.record().KeepHDR, "the stored row carries it too")
	}
}

// archiveFields are valid profile form values.
//
// Returns:
//   - fields: A valid set of raw profile values.
func archiveFields() ProfileFields {
	return ProfileFields{
		Name:      " Archive ",
		CRF:       "18",
		Preset:    "slow",
		AudioKbps: "320",
		MaxWidth:  "3840",
	}
}

// fieldsWith copies the valid values and applies edit to the copy.
//
// Parameters:
//   - t: The test the values belong to.
//   - edit: Change applied to the copy.
//
// Returns:
//   - fields: The edited values.
func fieldsWith(t *testing.T, edit func(*ProfileFields)) ProfileFields {
	t.Helper()

	fields := archiveFields()
	edit(&fields)

	return fields
}

func TestProfileName(t *testing.T) {
	t.Parallel()

	options := []ProfileOption{
		{ID: "profile-1080p", Name: "1080p", IsDefault: true},
		{ID: "archive", Name: "Archive", IsDefault: false},
	}

	assert.Equal(t, "Archive", ProfileName("archive", options))
	assert.Equal(t, "low", ProfileName("low", options),
		"a profile that is not offered is shown as its own id")
	assert.Empty(t, ProfileName("", nil))
}

// TestResolveProfile covers the profile a new clip carries: the stored
// default for an unnamed quality, a stored profile as it stands, and a refusal
// for any other id, including an old built-in id no profile carries now.
func TestResolveProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		give    func(t *testing.T, store *database.DB) string
		want    func(t *testing.T, store *database.DB) string
		wantErr error
	}{
		{
			name: "an unnamed quality takes the stored default",
			give: func(*testing.T, *database.DB) string { return "" },
			want: func(t *testing.T, store *database.DB) string {
				t.Helper()

				return builtinID(t, store, "1080p")
			},
		},
		{
			name: "a stored profile is taken as it stands",
			give: func(t *testing.T, store *database.DB) string {
				t.Helper()

				return builtinID(t, store, "4K")
			},
			want: func(t *testing.T, store *database.DB) string {
				t.Helper()

				return builtinID(t, store, "4K")
			},
		},
		{
			name:    "an old built-in id is rejected",
			give:    func(*testing.T, *database.DB) string { return "low" },
			wantErr: ErrUnknownProfile,
		},
		{
			name:    "an id no stored profile has is rejected",
			give:    func(*testing.T, *database.DB) string { return "no-such-profile" },
			wantErr: ErrUnknownProfile,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store, err := database.New(filepath.Join(t.TempDir(), "profile.db"))
			require.NoError(t, err)

			t.Cleanup(func() { _ = store.Close() })

			got, resolveErr := ResolveProfile(t.Context(), store, test.give(t, store))

			if test.wantErr != nil {
				require.ErrorIs(t, resolveErr, test.wantErr)

				return
			}

			require.NoError(t, resolveErr)
			assert.Equal(t, test.want(t, store), got)
		})
	}
}
