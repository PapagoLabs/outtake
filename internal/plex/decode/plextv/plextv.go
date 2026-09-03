// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:tagliatelle // Plex XML mixes camelCase attributes and PascalCase elements.
package plextv

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Media is a plex.tv XML media listing entry.
type Media struct {
	RatingKey string `xml:"ratingKey,attr"`
	Key       string `xml:"key,attr"`
	Title     string `xml:"title,attr"`
	Duration  int64  `xml:"duration,attr"`
	Thumb     string `xml:"thumb,attr"`
	Type      string `xml:"type,attr"`
}

// Session is a plex.tv XML playback session entry.
type Session struct {
	Info       sessionInfo `xml:"Session"`
	RatingKey  string      `xml:"ratingKey,attr"`
	Key        string      `xml:"key,attr"`
	Title      string      `xml:"title,attr"`
	Type       string      `xml:"type,attr"`
	Duration   int64       `xml:"duration,attr"`
	ViewOffset int64       `xml:"viewOffset,attr"`
	Thumb      string      `xml:"thumb,attr"`
}

// sessionInfo is the nested Session element.
type sessionInfo struct {
	ID string `xml:"id,attr"`
}

// Device is a plex.tv XML device from /api/resources.
type Device struct {
	Name        string       `xml:"name,attr"`
	Address     string       `xml:"address,attr"`
	Port        int          `xml:"port,attr"`
	AccessToken string       `xml:"accessToken,attr"`
	Connection  []Connection `xml:"Connection"`
}

// Connection is a plex.tv XML device connection.
type Connection struct {
	URI      string `xml:"uri,attr"`
	Address  string `xml:"address,attr"`
	Port     int    `xml:"port,attr"`
	Protocol string `xml:"protocol,attr"`
	Local    int    `xml:"local,attr"`
}

// Directory is a plex.tv XML directory listing entry.
type Directory struct {
	Key   string `xml:"key,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
}

// deviceResponse is the /api/resources envelope.
type deviceResponse struct {
	XMLName xml.Name `xml:"MediaContainer"`
	Device  []Device `xml:"Device"`
}

// searchResponse is the /search envelope.
type searchResponse struct {
	XMLName   xml.Name    `xml:"MediaContainer"`
	Video     []Media     `xml:"Video"`
	Directory []Directory `xml:"Directory"`
}

// sessionResponse is the /status/sessions envelope.
type sessionResponse struct {
	XMLName xml.Name  `xml:"MediaContainer"`
	Video   []Session `xml:"Video"`
}

// MetadataKeyPrefix is the PMS metadata key prefix stripped by ID.
const metadataKeyPrefix = "/library/metadata/"

// Devices unmarshals a plex.tv device-discovery envelope.
//
// Parameters:
//   - body: Raw XML bytes.
//
// Returns:
//   - devices: Discovered devices.
//   - err: Non-nil when body is not valid plex.tv XML.
func Devices(body []byte) ([]Device, error) {
	var data deviceResponse

	err := xml.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("decode devices: %w", err)
	}

	return data.Device, nil
}

// Search unmarshals a plex.tv search envelope.
//
// Parameters:
//   - body: Raw XML bytes.
//
// Returns:
//   - videos: Video hits.
//   - directories: Directory hits.
//   - err: Non-nil when body is not valid plex.tv XML.
func Search(body []byte) ([]Media, []Directory, error) {
	var data searchResponse

	err := xml.Unmarshal(body, &data)
	if err != nil {
		return nil, nil, fmt.Errorf("decode search: %w", err)
	}

	return data.Video, data.Directory, nil
}

// Sessions unmarshals a plex.tv sessions envelope.
//
// Parameters:
//   - body: Raw XML bytes.
//
// Returns:
//   - sessions: Playback sessions.
//   - err: Non-nil when body is not valid plex.tv XML.
func Sessions(body []byte) ([]Session, error) {
	var data sessionResponse

	err := xml.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("decode sessions: %w", err)
	}

	return data.Video, nil
}

// ID prefers ratingKey, then the metadata id in key.
//
// Returns:
//   - id: The metadata identifier, or empty when the key prefix is missing.
func (entry Media) ID() string {
	if entry.RatingKey != "" {
		return entry.RatingKey
	}

	id, ok := strings.CutPrefix(entry.Key, metadataKeyPrefix)
	if !ok {
		return ""
	}

	id = strings.TrimSuffix(id, "/")
	if slash := strings.Index(id, "/"); slash >= 0 {
		id = id[:slash]
	}

	return id
}

// Media converts a session entry into a media listing entry.
//
// Returns:
//   - media: A Media value with the session identity fields.
func (entry Session) Media() Media {
	return Media{
		RatingKey: entry.RatingKey,
		Key:       entry.Key,
		Title:     entry.Title,
		Duration:  entry.Duration,
		Thumb:     entry.Thumb,
		Type:      entry.Type,
	}
}

// PlaybackID returns the nested session identifier.
//
// Returns:
//   - id: The plex.tv session id.
func (entry Session) PlaybackID() string {
	return entry.Info.ID
}
