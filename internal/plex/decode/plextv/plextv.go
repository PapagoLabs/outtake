// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:tagliatelle // Plex XML mixes camelCase attributes and PascalCase elements.
package plextv

import (
	"encoding/xml"
	"fmt"

	"github.com/PapagoLabs/outtake/internal/plex/decode/key"
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

// Device is a plex.tv XML device from /api/resources.
type Device struct {
	Name             string       `xml:"name,attr"`
	ClientIdentifier string       `xml:"clientIdentifier,attr"`
	Provides         string       `xml:"provides,attr"`
	Owned            int          `xml:"owned,attr"`
	Address          string       `xml:"address,attr"`
	Port             int          `xml:"port,attr"`
	AccessToken      string       `xml:"accessToken,attr"`
	Connection       []Connection `xml:"Connection"`
}

// Connection is a plex.tv XML device connection.
type Connection struct {
	URI      string `xml:"uri,attr"`
	Address  string `xml:"address,attr"`
	Port     int    `xml:"port,attr"`
	Protocol string `xml:"protocol,attr"`
	Local    int    `xml:"local,attr"`
	Relay    int    `xml:"relay,attr"`
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

// ID prefers ratingKey, then the metadata id in key.
//
// Returns:
//   - id: The metadata identifier, or empty when the key prefix is missing.
func (entry Media) ID() string {
	if entry.RatingKey != "" {
		return entry.RatingKey
	}

	return key.ID(entry.Key)
}
