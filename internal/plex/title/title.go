// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package title

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PapagoLabs/outtake/internal/plex/page"
)

// TypeEpisode is a TV episode media type.
const TypeEpisode = "episode"

// Display returns a user-facing title, including show and episode codes.
//
// Parameters:
//   - item: Plex media metadata item.
//
// Returns:
//   - value: A user-facing title, including show and episode codes.
func Display(item page.MediaItem) string {
	if item.Type == TypeEpisode {
		return episodeDisplay(item)
	}

	if item.Year > 0 && item.Title != "" {
		return item.Title + " (" + strconv.Itoa(item.Year) + ")"
	}

	return item.Title
}

// EpisodeCode formats a season and episode number as S01E03.
//
// Parameters:
//   - season: TV season number.
//   - episode: TV episode number.
//
// Returns:
//   - value: Result value; zero or empty when unavailable.
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

// episodeDisplay returns the episode display.
//
// Parameters:
//   - item: Plex media metadata item.
//
// Returns:
//   - value: The episode display.
func episodeDisplay(item page.MediaItem) string {
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
