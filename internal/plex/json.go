// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"fmt"

	"github.com/PapagoLabs/outtake/internal/plex/decode/pms"
)

// decodePMS unmarshals a documented PMS JSON MediaContainer envelope.
//
// Parameters:
//   - body: Raw PMS JSON bytes.
//
// Returns:
//   - container: The decoded MediaContainer.
//   - err: Non-nil when body is empty or not valid PMS JSON.
func decodePMS(body []byte) (pms.Container, error) {
	if len(body) == 0 {
		return pms.Container{}, errEmptyBody
	}

	container, err := pms.Decode(body)
	if err != nil {
		return pms.Container{}, fmt.Errorf("pms: %w", err)
	}

	return container, nil
}

// metadataToItem maps a PMS metadata object onto a MediaItem.
//
// Parameters:
//   - meta: Decoded PMS metadata.
//   - libraryTitle: Owning library title, or empty.
//
// Returns:
//   - item: The mapped media item.
func metadataToItem(meta pms.Metadata, libraryTitle string) MediaItem {
	return MediaItem{
		ID:               meta.ID(),
		Title:            meta.Title,
		Type:             MapPlexType(meta.Type),
		Duration:         float64(meta.Duration) / scaleMsToS,
		ThumbPath:        meta.Thumb,
		LibraryTitle:     libraryTitle,
		LibraryID:        string(meta.LibrarySectionID),
		Year:             meta.Year,
		Index:            meta.Index,
		ParentIndex:      meta.ParentIndex,
		ParentID:         string(meta.ParentRatingKey),
		ParentTitle:      meta.ParentTitle,
		GrandparentID:    string(meta.GrandparentRatingKey),
		GrandparentTitle: meta.GrandparentTitle,
	}
}

// metadataItems converts documented Metadata arrays onto MediaItems.
//
// Parameters:
//   - metas: Decoded PMS metadata rows.
//   - libraryTitle: Owning library title, or empty.
//
// Returns:
//   - items: Mapped media items with a metadata identity.
func metadataItems(metas []pms.Metadata, libraryTitle string) []MediaItem {
	items := make([]MediaItem, 0, len(metas))
	for index := range metas {
		meta := &metas[index]
		if !meta.HasMetadata() {
			continue
		}

		items = append(items, metadataToItem(*meta, libraryTitle))
	}

	return items
}

// firstMediaFile returns the first on-disk part path from metadata.
//
// Parameters:
//   - meta: Decoded PMS metadata.
//
// Returns:
//   - path: The first non-empty part file, or empty when none exist.
func firstMediaFile(meta pms.Metadata) string {
	return meta.File()
}
