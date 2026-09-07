// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package pms

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// envelope is the documented PMS JSON root object.
//
//nolint:tagliatelle // PMS JSON uses PascalCase MediaContainer keys.
type envelope struct {
	MediaContainer Container `json:"MediaContainer"`
}

// Container is MediaContainer as documented by the PMS OpenAPI spec.
//
//nolint:tagliatelle // PMS JSON uses PascalCase Directory/Metadata/Hub keys.
type Container struct {
	Directory         []Section  `json:"Directory"`
	Metadata          []Metadata `json:"Metadata"`
	Hub               []Hub      `json:"Hub"`
	MachineIdentifier string     `json:"machineIdentifier"`
	Version           string     `json:"version"`
	Size              int        `json:"size"`
	TotalSize         int        `json:"totalSize"`
	Offset            int        `json:"offset"`
}

// Section is a library section from GET /library/sections/all.
type Section struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Thumb     string `json:"thumb"`
	Composite string `json:"composite"`
	Size      int    `json:"size"`
	LeafCount int    `json:"leafCount"`
}

// Hub is a search hub from GET /hubs/search.
//
//nolint:tagliatelle // PMS JSON uses PascalCase Metadata keys.
type Hub struct {
	Title    string     `json:"title"`
	Type     string     `json:"type"`
	Metadata []Metadata `json:"Metadata"`
}

// Metadata is a metadata item from the documented metadata schema.
//
//nolint:tagliatelle // PMS JSON uses PascalCase Media/Session keys.
type Metadata struct {
	RatingKey            flexString `json:"ratingKey"`
	Key                  string     `json:"key"`
	Title                string     `json:"title"`
	Type                 string     `json:"type"`
	Duration             int64      `json:"duration"`
	ViewOffset           int64      `json:"viewOffset"`
	Thumb                string     `json:"thumb"`
	Year                 int        `json:"year"`
	Index                int        `json:"index"`
	ParentIndex          int        `json:"parentIndex"`
	ParentRatingKey      flexString `json:"parentRatingKey"`
	ParentTitle          string     `json:"parentTitle"`
	GrandparentRatingKey flexString `json:"grandparentRatingKey"`
	GrandparentTitle     string     `json:"grandparentTitle"`
	LibrarySectionID     flexString `json:"librarySectionID"`
	TitleSort            string     `json:"titleSort"`
	AddedAt              int64      `json:"addedAt"`
	Media                []media    `json:"Media"`
	Session              session    `json:"Session"`
}

// media is a media version on a metadata item.
//
//nolint:tagliatelle // PMS JSON uses PascalCase Part keys.
type media struct {
	Part []part `json:"Part"`
}

// part is a media part with the on-disk file path.
type part struct {
	File string `json:"file"`
}

// session is playback session info attached to session metadata.
type session struct {
	ID string `json:"id"`
}

// flexString accepts JSON strings or numbers, matching PMS ratingKey values.
type flexString string

const (
	// DecimalBase is the numeric base used when parsing ratingKey integers.
	decimalBase = 10

	// IntBitSize is the bit size used when parsing ratingKey integers.
	intBitSize = 64

	// MetadataKeyPrefix is the PMS metadata key prefix stripped by ID.
	metadataKeyPrefix = "/library/metadata/"
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

// Decode unmarshals a documented PMS JSON MediaContainer envelope.
//
// Parameters:
//   - body: Raw PMS JSON bytes.
//
// Returns:
//   - container: The decoded MediaContainer.
//   - err: Non-nil when body is not valid PMS JSON.
func Decode(body []byte) (Container, error) {
	var env envelope

	err := json.Unmarshal(body, &env)
	if err != nil {
		return Container{}, fmt.Errorf("decode json: %w", err)
	}

	return env.MediaContainer, nil
}

// File returns the first on-disk part path from metadata.
//
// Returns:
//   - path: The first non-empty part file, or empty when none exist.
func (meta Metadata) File() string {
	for mediaIndex := range meta.Media {
		for partIndex := range meta.Media[mediaIndex].Part {
			mediaPart := meta.Media[mediaIndex].Part[partIndex]
			if mediaPart.File != "" {
				return mediaPart.File
			}
		}
	}

	return ""
}

// HasMetadata reports whether the item has a ratingKey or metadata key.
//
// Returns:
//   - ok: True when the item can map onto a MediaItem.
func (meta Metadata) HasMetadata() bool {
	_, hasPrefix := strings.CutPrefix(meta.Key, metadataKeyPrefix)

	return meta.RatingKey != "" || hasPrefix
}

// ID prefers ratingKey, then the metadata id in key.
//
// Returns:
//   - id: The PMS metadata identifier.
func (meta Metadata) ID() string {
	if meta.RatingKey != "" {
		return string(meta.RatingKey)
	}

	id, ok := strings.CutPrefix(meta.Key, metadataKeyPrefix)
	if !ok {
		return ""
	}

	id = strings.TrimSuffix(id, "/")
	if slash := strings.Index(id, "/"); slash >= 0 {
		id = id[:slash]
	}

	return id
}
