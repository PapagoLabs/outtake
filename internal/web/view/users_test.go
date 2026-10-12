// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/clip/quota"
)

// TestLimitFieldsWriteEachLimit covers the limits forms' values.
//
// The global form shows every limit, and a user's form leaves empty each
// limit the user takes from the global ones.
func TestLimitFieldsWriteEachLimit(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		LimitFields{Active: "2", Weekly: "100", StorageGB: "25", ClipSeconds: ""},
		GlobalLimitFields(quota.Defaults()),
		"a limit that is off is left empty",
	)
	assert.Equal(t, LimitFields{}, GlobalLimitFields(quota.Limits{}))

	active := 3
	storage := int64(2_500_000_000)
	clip := 90 * time.Second

	assert.Equal(t,
		LimitFields{Active: "3", Weekly: "", StorageGB: "2.5", ClipSeconds: "90"},
		OverrideFields(quota.Override{Active: &active, Storage: &storage, ClipLength: &clip}),
	)
}

// TestUsageLabelCountsRendersAndStorage covers the usage line under a user.
func TestUsageLabelCountsRendersAndStorage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "1 render this week · 512 bytes", UsageLabel(1, 512))
	assert.Equal(t, "3 renders this week · 1.2 GB", UsageLabel(3, 1_200_000_000))
}

// TestLimitPlaceholdersNameWhatAnEmptyFieldMeans covers the placeholders of
// the limits forms.
//
// A member's empty field shows the global value, and a field whose global
// limit is off says so.
func TestLimitPlaceholdersNameWhatAnEmptyFieldMeans(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		LimitFields{
			Active: PlaceholderNoLimit, Weekly: PlaceholderNoLimit,
			StorageGB: PlaceholderNoLimit, ClipSeconds: PlaceholderClipMaximum,
		},
		LimitPlaceholders(LimitFields{}),
	)
	assert.Equal(
		t,
		LimitFields{
			Active:      "2",
			Weekly:      "100",
			StorageGB:   "25",
			ClipSeconds: PlaceholderClipMaximum,
		},
		LimitPlaceholders(GlobalLimitFields(quota.Defaults())),
	)
}
