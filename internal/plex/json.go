// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// pmsEnvelope is the documented PMS JSON root object.
//
//nolint:tagliatelle // PMS JSON uses PascalCase MediaContainer keys.
type pmsEnvelope struct {
	MediaContainer pmsContainer `json:"MediaContainer"`
}

// pmsContainer is MediaContainer as documented by the PMS OpenAPI spec.
//
//nolint:tagliatelle // PMS JSON uses PascalCase Directory/Metadata/Hub keys.
type pmsContainer struct {
	Directory         []pmsSection  `json:"Directory"`
	Metadata          []pmsMetadata `json:"Metadata"`
	Hub               []pmsHub      `json:"Hub"`
	MachineIdentifier string        `json:"machineIdentifier"`
	Version           string        `json:"version"`
}

// pmsSection is a library section from GET /library/sections/all.
type pmsSection struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// pmsHub is a search hub from GET /hubs/search.
//
//nolint:tagliatelle // PMS JSON uses PascalCase Metadata keys.
type pmsHub struct {
	Title    string        `json:"title"`
	Type     string        `json:"type"`
	Metadata []pmsMetadata `json:"Metadata"`
}

// pmsMetadata is a metadata item from the documented metadata schema.
//
//nolint:tagliatelle // PMS JSON uses PascalCase Media/Session keys.
type pmsMetadata struct {
	RatingKey  flexString `json:"ratingKey"`
	Key        string     `json:"key"`
	Title      string     `json:"title"`
	Type       string     `json:"type"`
	Duration   int64      `json:"duration"`
	ViewOffset int64      `json:"viewOffset"`
	Thumb      string     `json:"thumb"`
	Media      []pmsMedia `json:"Media"`
	Session    pmsSession `json:"Session"`
}

// pmsMedia is a media version on a metadata item.
//
//nolint:tagliatelle // PMS JSON uses PascalCase Part keys.
type pmsMedia struct {
	Part []pmsPart `json:"Part"`
}

// pmsPart is a media part with the on-disk file path.
type pmsPart struct {
	File string `json:"file"`
}

// pmsSession is playback session info attached to session metadata.
type pmsSession struct {
	ID string `json:"id"`
}

// flexString accepts JSON strings or numbers, matching PMS ratingKey values.
type flexString string

const (
	// DecimalBase is the numeric base used when parsing ratingKey integers.
	decimalBase = 10

	// IntBitSize is the bit size used when parsing ratingKey integers.
	intBitSize = 64
)

// UnmarshalJSON implements [json.Unmarshaler].
func (value *flexString) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		*value = ""

		return nil
	}

	if len(raw) > 0 && raw[0] == '"' {
		var text string

		err := json.Unmarshal(raw, &text)
		if err != nil {
			return fmt.Errorf("decode string: %w", err)
		}

		*value = flexString(text)

		return nil
	}

	n, err := strconv.ParseInt(string(raw), decimalBase, intBitSize)
	if err != nil {
		return fmt.Errorf("decode number: %w", err)
	}

	*value = flexString(strconv.FormatInt(n, decimalBase))

	return nil
}

// decodePMS unmarshals a documented PMS JSON MediaContainer envelope.
func decodePMS(body []byte) (pmsContainer, error) {
	if len(body) == 0 {
		return pmsContainer{}, errEmptyBody
	}

	var env pmsEnvelope

	err := json.Unmarshal(body, &env)
	if err != nil {
		return pmsContainer{}, fmt.Errorf("decode json: %w", err)
	}

	return env.MediaContainer, nil
}

// metadataToItem maps a PMS metadata object onto a MediaItem.
func metadataToItem(meta pmsMetadata, libraryTitle string) MediaItem {
	return MediaItem{
		ID:           metadataID(meta),
		Title:        meta.Title,
		Type:         MapPlexType(meta.Type),
		Duration:     float64(meta.Duration) / scaleMsToS,
		ThumbPath:    meta.Thumb,
		LibraryTitle: libraryTitle,
	}
}

// metadataID prefers ratingKey, then the metadata id in key.
func metadataID(meta pmsMetadata) string {
	if meta.RatingKey != "" {
		return string(meta.RatingKey)
	}

	id := strings.TrimPrefix(meta.Key, "/library/metadata/")

	id = strings.TrimSuffix(id, "/")
	if slash := strings.Index(id, "/"); slash >= 0 {
		id = id[:slash]
	}

	return id
}

// metadataItems converts documented Metadata arrays onto MediaItems.
func metadataItems(metas []pmsMetadata, libraryTitle string) []MediaItem {
	items := make([]MediaItem, 0, len(metas))
	for index := range metas {
		meta := &metas[index]
		if meta.RatingKey == "" && !strings.Contains(meta.Key, "/metadata/") {
			continue
		}

		items = append(items, metadataToItem(*meta, libraryTitle))
	}

	return items
}

// firstMediaFile returns the first on-disk part path from metadata.
func firstMediaFile(meta pmsMetadata) string {
	for mediaIndex := range meta.Media {
		for partIndex := range meta.Media[mediaIndex].Part {
			part := meta.Media[mediaIndex].Part[partIndex]
			if part.File != "" {
				return part.File
			}
		}
	}

	return ""
}
