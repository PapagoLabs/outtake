// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

// LibraryItem is a Plex library in the sidebar and media browse grid.
type LibraryItem struct {
	ID        string
	Title     string
	Type      string
	ThumbPath string
}

// MediaItem is one title or container in the media library grid.
type MediaItem struct {
	ID           string
	Title        string
	Type         string
	Duration     float64
	ThumbPath    string
	Browsable    bool
	BrowseURL    string
	Year         int
	Index        int
	ParentIndex  int
	ShowTitle    string
	EpisodeLabel string
}

// Crumb is one step in the media library trail.
type Crumb struct {
	Title string
	URL   string
}

// MediaProps is the media library page and its HTMX results fragment.
type MediaProps struct {
	Items       []MediaItem
	Libraries   []LibraryItem
	Crumbs      []Crumb
	Query       string
	LibraryID   string
	ParentID    string
	ParentTitle string
	UpID        string
	UpTitle     string
	Start       int
	Total       int
	PageSize    int
	HasServer   bool
}
