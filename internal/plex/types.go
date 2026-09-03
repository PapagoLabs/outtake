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
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	ThumbPath string `json:"thumbPath,omitempty"`
}

// MediaItem represents a media item in Plex.
type MediaItem struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Type             string  `json:"type"`
	Duration         float64 `json:"duration"`
	ThumbPath        string  `json:"thumbPath"`
	LibraryTitle     string  `json:"libraryTitle"`
	LibraryID        string  `json:"libraryId,omitempty"`
	Year             int     `json:"year,omitempty"`
	Index            int     `json:"index,omitempty"`
	ParentIndex      int     `json:"parentIndex,omitempty"`
	ParentID         string  `json:"parentId,omitempty"`
	ParentTitle      string  `json:"parentTitle,omitempty"`
	GrandparentID    string  `json:"grandparentId,omitempty"`
	GrandparentTitle string  `json:"grandparentTitle,omitempty"`
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

	// TypeEpisode is a TV episode.
	TypeEpisode = "episode"

	// TypeSeason is a season container.
	TypeSeason = "season"

	// TypeArtist is a music artist container.
	TypeArtist = "artist"

	// TypeAlbum is an album container.
	TypeAlbum = "album"
)

// EmptyServer returns a Server with every exported field set to its zero value.
//
// Returns:
//   - server: A Server ready to fill or return as a missing-server sentinel.
func EmptyServer() Server {
	return Server{
		Name:    "",
		Address: "",
		Port:    0,
		Token:   "",
		Scheme:  "",
		Local:   false,
	}
}

// IsContainerType reports whether the metadata type has children to browse.
func IsContainerType(mediaType string) bool {
	switch mediaType {
	case TypeShow, TypeSeason, TypeArtist, TypeAlbum:
		return true
	default:
		return false
	}
}
