// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// TestMaxPreviewWidthDefaultsTo1080p covers the width before one is chosen:
// with no database, with nothing stored, and with a stored value the settings
// do not offer.
func TestMaxPreviewWidthDefaultsTo1080p(t *testing.T) {
	t.Parallel()

	assert.Equal(t, clip.OutputWidth1080p, MaxPreviewWidth(t.Context(), nil))

	db, err := database.New(t.TempDir() + "/playback.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	assert.Equal(t, clip.OutputWidth1080p, MaxPreviewWidth(t.Context(), db))

	require.NoError(t, db.SaveSetting(t.Context(), settingMaxPreviewWidth, "2560"))
	assert.Equal(t, clip.OutputWidth1080p, MaxPreviewWidth(t.Context(), db),
		"a width the settings do not offer is not used")
}

// TestSaveMaxPreviewWidthStoresAnOfferedWidth covers the setting a user
// chooses: each offered width, 720p through 4K, is stored and read back, and
// any other is refused.
func TestSaveMaxPreviewWidthStoresAnOfferedWidth(t *testing.T) {
	t.Parallel()

	db, err := database.New(t.TempDir() + "/playback.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, width := range []int{clip.OutputWidth2160p, clip.OutputWidth720p} {
		require.NoError(t, SaveMaxPreviewWidth(t.Context(), db, strconv.Itoa(width)))
		assert.Equal(t, width, MaxPreviewWidth(t.Context(), db))
	}

	for _, raw := range []string{"2560", "wide", ""} {
		require.ErrorIs(
			t,
			SaveMaxPreviewWidth(t.Context(), db, raw),
			ErrUnknownMaxPreviewWidth,
			raw,
		)
	}

	assert.Equal(
		t,
		clip.OutputWidth720p,
		MaxPreviewWidth(t.Context(), db),
		"a refused width changes nothing",
	)
}
