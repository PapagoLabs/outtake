// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
)

var testProfileOptions = []profile.ProfileOption{
	{ID: "profile-1", Name: "Podcast"},
	{ID: "profile-2", Name: "Archive", IsDefault: true},
}

func TestNewClipItemMapsEveryField(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, time.September, 3, 4, 32, 20, 0, time.UTC)
	limit := 60 * time.Second

	job := &clip.Job{
		ID:         "clip-1",
		Type:       clip.TypeGIF,
		Name:       "Opening",
		MediaID:    "42",
		MediaTitle: "Movie",
		Quality:    "profile-1",

		StartTime:     12 * time.Second,
		Duration:      4250 * time.Millisecond,
		AudioIndex:    2,
		CropBlackBars: true,
		WebSafeColor:  true,
		PreserveHDR:   true,

		CreatedAt: created,
		UpdatedAt: created.Add(time.Minute),
		Width:     640,
		FPS:       12, OutputPath: "/clips/clip-1.mp4",

		Status:   clip.StatusCompleted,
		Progress: 40,
		Error:    "ffmpeg exited 1",
	}

	item := NewClipItem(job, testProfileOptions, limit, true)

	assert.Equal(t, ClipItem{
		ID:            "clip-1",
		Name:          "Opening",
		MediaID:       "42",
		MediaTitle:    "Movie",
		ClipType:      clip.TypeGIF,
		Status:        clip.StatusCompleted,
		Progress:      40,
		CreatedAt:     "Sep 3, 2026 4:32 AM",
		FileVersion:   strconv.FormatInt(created.Add(time.Minute).UnixMilli(), fileVersionBase),
		StartTime:     12 * time.Second,
		Duration:      4250 * time.Millisecond,
		Quality:       "profile-1",
		ProfileName:   "Podcast",
		Profiles:      testProfileOptions,
		FileExists:    true,
		AudioIndex:    2,
		CropBlackBars: true,
		WebSafeColor:  true,
		PreserveHDR:   true,
		Width:         640,
		FPS:           12,
		MaxDur:        limit,
		Error:         "ffmpeg exited 1",
	}, item)
}

func TestNewClipItemResolvesTheProfileName(t *testing.T) {
	t.Parallel()

	offered := NewClipItem(&clip.Job{Quality: "profile-2"}, testProfileOptions, 0, false)
	assert.Equal(t, "Archive", offered.ProfileName)

	unknown := NewClipItem(&clip.Job{Quality: "profile-9"}, testProfileOptions, 0, false)
	assert.Equal(t, "profile-9", unknown.ProfileName,
		"a clip whose profile is no longer offered is labeled by its own id")
}

func TestNewClipItemsChecksEachOutputPath(t *testing.T) {
	t.Parallel()

	secondOutput := "/clips/clip-2.gif"
	jobs := []*clip.Job{
		{ID: "clip-1", OutputPath: "/clips/clip-1.mp4"},
		{ID: "clip-2", OutputPath: secondOutput},
	}

	var asked []string

	items := NewClipItems(jobs, testProfileOptions, 0, func(path string) bool {
		asked = append(asked, path)

		return path == "/clips/clip-1.mp4"
	})

	assert.Equal(t, []string{"/clips/clip-1.mp4", secondOutput}, asked,
		"every row is checked against its own output, in the order the clips were given")
	require.Len(t, items, 2)
	assert.True(t, items[0].FileExists)
	assert.False(t, items[1].FileExists)
	assert.Equal(t, "clip-1", items[0].ID)
	assert.Equal(t, "clip-2", items[1].ID)
}

func TestNewClipItemsAreEmptyForNoClips(t *testing.T) {
	t.Parallel()

	asked := false
	items := NewClipItems(nil, testProfileOptions, 0, func(string) bool {
		asked = true

		return false
	})

	assert.Empty(t, items)
	assert.False(t, asked, "no clips means no output to look for")
}
