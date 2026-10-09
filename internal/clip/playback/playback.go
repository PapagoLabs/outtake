// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/store/database"
)

// settingMaxPreviewWidth is the app_settings name the maximum preview width
// is stored under.
const settingMaxPreviewWidth = "max_preview_width"

// DefaultMaxPreviewWidth is the maximum preview width until one is chosen.
const DefaultMaxPreviewWidth = clip.OutputWidth1080p

// ErrUnknownMaxPreviewWidth reports a maximum preview width the app does not
// offer.
var ErrUnknownMaxPreviewWidth = errors.New(
	"maximum preview resolution must be 720p, 1080p, or 4K",
)

// MaxPreviewWidths lists the maximum preview widths the settings offer,
// narrowest first.
//
// Returns:
//   - widths: A copy of the offered widths.
func MaxPreviewWidths() []int {
	return []int{clip.OutputWidth720p, clip.OutputWidth1080p, clip.OutputWidth2160p}
}

// MaxPreviewWidth reads the widest any preview is rendered: a form's preview
// before a clip is saved, and the SDR version a clip that keeps HDR renders
// for its card. A narrower source keeps its own width.
//
// Parameters:
//   - ctx: Request scope for the read.
//   - db: Settings store; may be nil.
//
// Returns:
//   - width: The stored width, or DefaultMaxPreviewWidth when none is stored,
//     the stored value is not one the settings offer, or it cannot be read.
func MaxPreviewWidth(ctx context.Context, db *database.DB) int {
	if db == nil {
		return DefaultMaxPreviewWidth
	}

	raw, found, err := db.Setting(ctx, settingMaxPreviewWidth)
	if err != nil || !found {
		return DefaultMaxPreviewWidth
	}

	width, err := ParseMaxPreviewWidth(raw)
	if err != nil {
		return DefaultMaxPreviewWidth
	}

	return width
}

// ParseMaxPreviewWidth reads a maximum preview width as stored or as a form
// posts it.
//
// Parameters:
//   - raw: The width in pixels, such as "1920".
//
// Returns:
//   - width: The width.
//   - err: ErrUnknownMaxPreviewWidth when raw is not a width the settings
//     offer.
func ParseMaxPreviewWidth(raw string) (int, error) {
	width, err := strconv.Atoi(raw)
	if err != nil || !slices.Contains(MaxPreviewWidths(), width) {
		return 0, ErrUnknownMaxPreviewWidth
	}

	return width, nil
}

// SaveMaxPreviewWidth stores the widest previews are rendered from now on.
//
// Parameters:
//   - ctx: Request scope for the write.
//   - db: Settings store.
//   - raw: The width a form posted.
//
// Returns:
//   - err: ErrUnknownMaxPreviewWidth for a width the settings do not offer,
//     or the wrapped write failure.
func SaveMaxPreviewWidth(ctx context.Context, db *database.DB, raw string) error {
	width, err := ParseMaxPreviewWidth(raw)
	if err != nil {
		//nolint:wrapcheck // The message is shown to the user as it is.
		return err
	}

	err = db.SaveSetting(ctx, settingMaxPreviewWidth, strconv.Itoa(width))
	if err != nil {
		return fmt.Errorf("save maximum preview width: %w", err)
	}

	return nil
}
