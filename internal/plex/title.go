// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"fmt"
	"strconv"
	"strings"
)

// DisplayTitle returns a user-facing title, including show and episode codes.
//
// Returns:
//   - title: The label shown in lists, sessions, and page headings.
func (item MediaItem) DisplayTitle() string {
	if item.Type == TypeEpisode {
		return episodeDisplayTitle(item)
	}

	if item.Year > 0 && item.Title != "" {
		return item.Title + " (" + strconv.Itoa(item.Year) + ")"
	}

	return item.Title
}

// EpisodeCode formats a season and episode number as S01E03.
//
// Parameters:
//   - season: 1-based season number, or 0 when unknown.
//   - episode: 1-based episode number, or 0 when unknown.
//
// Returns:
//   - code: SxxExx, or empty when both values are missing.
func EpisodeCode(season, episode int) string {
	if season <= 0 && episode <= 0 {
		return ""
	}

	if season <= 0 {
		return fmt.Sprintf("E%02d", episode)
	}

	if episode <= 0 {
		return fmt.Sprintf("S%02d", season)
	}

	return fmt.Sprintf("S%02dE%02d", season, episode)
}

// SameConnection reports whether two servers share scheme, host, and port.
func SameConnection(left, right Server) bool {
	return left.Scheme == right.Scheme && left.Address == right.Address && left.Port == right.Port
}

// episodeDisplayTitle joins show, episode code, and episode title.
func episodeDisplayTitle(item MediaItem) string {
	var parts []string

	if item.GrandparentTitle != "" {
		parts = append(parts, item.GrandparentTitle)
	}

	if code := EpisodeCode(item.ParentIndex, item.Index); code != "" {
		parts = append(parts, code)
	}

	if item.Title != "" {
		parts = append(parts, item.Title)
	}

	return strings.Join(parts, " · ")
}
