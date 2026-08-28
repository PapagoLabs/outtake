// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

// Server represents a Plex server.
type Server struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Scheme  string `json:"scheme"`
	Local   bool   `json:"local"`
}

// Library represents a Plex library.
type Library struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// MediaItem represents a media item in Plex.
type MediaItem struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Type         string  `json:"type"`
	Duration     float64 `json:"duration"`
	ThumbPath    string  `json:"thumbPath"`
	LibraryTitle string  `json:"libraryTitle"`
}

// Session represents a playback session.
type Session struct {
	ID         string    `json:"id"`
	MediaItem  MediaItem `json:"mediaItem"`
	Title      string    `json:"title"`
	Duration   float64   `json:"duration"`
	ViewOffset float64   `json:"viewOffset"`
}

// ServerIdentity represents the server identity.
type ServerIdentity struct {
	MachineIdentifier string `json:"machineIdentifier"`
	Version           string `json:"version"`
}

const (
	// TypeShow is a TV show container.
	TypeShow = "show"

	// TypeSeason is a season container.
	TypeSeason = "season"

	// TypeArtist is a music artist container.
	TypeArtist = "artist"

	// TypeAlbum is an album container.
	TypeAlbum = "album"
)

// IsContainerType reports whether the metadata type has children to browse.
func IsContainerType(mediaType string) bool {
	switch mediaType {
	case TypeShow, TypeSeason, TypeArtist, TypeAlbum:
		return true
	default:
		return false
	}
}
