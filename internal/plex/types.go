// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"github.com/PapagoLabs/outtake/internal/plex/page"
	"github.com/PapagoLabs/outtake/internal/plex/server"
)

// MediaItem is a Plex library or container item.
type MediaItem = page.MediaItem

// MediaPage is one page of library or container children.
type MediaPage = page.MediaPage

// LetterIndex is one first-character bucket from a library section.
type LetterIndex = page.LetterIndex

// Server is a Plex Media Server connection.
type Server = server.Server

// Library represents a Plex library.
type Library struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	ThumbPath string `json:"thumbPath,omitempty"`
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
//   - server: A Server with every exported field set to its zero value.
func EmptyServer() Server {
	return server.Empty()
}

// IsContainerType reports whether the metadata type has children to browse.
//
// Parameters:
//   - mediaType: Media type.
//
// Returns:
//   - ok: True when the metadata type has children to browse.
func IsContainerType(mediaType string) bool {
	switch mediaType {
	case TypeShow, TypeSeason, TypeArtist, TypeAlbum:
		return true
	default:
		return false
	}
}
