// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package media

import (
	"github.com/PapagoLabs/outtake/internal/media/crop"
)

// CropRect is an ffmpeg crop=W:H:X:Y rectangle.
type CropRect = crop.CropRect

// ParseCropdetect returns the last crop=W:H:X:Y from cropdetect logs.
//
// Parameters:
//   - output: ffmpeg stderr text from a cropdetect pass.
//
// Returns:
//   - crop: The last parsed rectangle, or a zero value.
//   - ok: True when a valid rectangle was found.
func ParseCropdetect(output string) (CropRect, bool) {
	return crop.ParseCropdetect(output)
}
