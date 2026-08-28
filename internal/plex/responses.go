// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:tagliatelle // Plex XML mixes camelCase attributes and PascalCase elements.
package plex

import (
	"encoding/xml"
)

// mediaEntry represents a single media entry.
type mediaEntry struct {
	RatingKey string `xml:"ratingKey,attr"`
	Key       string `xml:"key,attr"`
	Title     string `xml:"title,attr"`
	Duration  int64  `xml:"duration,attr"`
	Thumb     string `xml:"thumb,attr"`
	Type      string `xml:"type,attr"`
}

// sessionResponse represents the response from the Plex API for sessions.
type sessionResponse struct {
	XMLName xml.Name       `xml:"MediaContainer"`
	Video   []sessionEntry `xml:"Video"`
}

// sessionEntry represents a single session entry.
type sessionEntry struct {
	Session    sessionInfo `xml:"Session"`
	RatingKey  string      `xml:"ratingKey,attr"`
	Key        string      `xml:"key,attr"`
	Title      string      `xml:"title,attr"`
	Type       string      `xml:"type,attr"`
	Duration   int64       `xml:"duration,attr"`
	ViewOffset int64       `xml:"viewOffset,attr"`
}

// sessionInfo represents session information.
type sessionInfo struct {
	ID string `xml:"id,attr"`
}

// deviceResponse represents the response from the Plex API for device discovery.
type deviceResponse struct {
	XMLName xml.Name      `xml:"MediaContainer"`
	Device  []deviceEntry `xml:"Device"`
}

// deviceEntry represents a single device entry.
type deviceEntry struct {
	Name        string             `xml:"name,attr"`
	Address     string             `xml:"address,attr"`
	Port        int                `xml:"port,attr"`
	AccessToken string             `xml:"accessToken,attr"`
	Connection  []deviceConnection `xml:"Connection"`
}

// deviceConnection represents a device connection.
type deviceConnection struct {
	URI      string `xml:"uri,attr"`
	Address  string `xml:"address,attr"`
	Port     int    `xml:"port,attr"`
	Protocol string `xml:"protocol,attr"`
	Local    int    `xml:"local,attr"`
}

// directoryEntry represents a single directory entry.
type directoryEntry struct {
	Key   string `xml:"key,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
}

// searchResponse represents the response for search results.
type searchResponse struct {
	XMLName   xml.Name         `xml:"MediaContainer"`
	Video     []mediaEntry     `xml:"Video"`
	Directory []directoryEntry `xml:"Directory"`
}
