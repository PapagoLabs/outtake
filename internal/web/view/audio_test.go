// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/plex/library"
)

func TestAudioTrackLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "eng · dts · 7.1 · DTS:X 7.1", AudioTrackLabel(library.AudioStream{
		Index:    0,
		Codec:    "dts",
		Language: "eng",
		Title:    "DTS:X 7.1",
		Layout:   "7.1",
	}))
	assert.Equal(t, "Track 2", AudioTrackLabel(library.AudioStream{Index: 1}),
		"a track that says nothing about itself is still selectable by index")
	assert.Equal(t, "aac", AudioTrackLabel(library.AudioStream{Codec: "aac"}),
		"an undetermined language is not worth a label")
}

func TestAudioTrackOptionsKeepEveryTrackInProbeOrder(t *testing.T) {
	t.Parallel()

	options := AudioTrackOptions([]library.AudioStream{
		{Index: 0, Codec: undetermined, Language: undetermined, Title: undetermined},
		{Index: 1, Codec: "aac", Language: "eng", Layout: "Stereo"},
		{Index: 2, Codec: "aac", Language: "jpn", Layout: "Stereo"},
	})

	require.Len(t, options, 3,
		"a track is only dropped when the whole list says nothing about any of them")
	assert.Equal(t, 0, options[0].Index, "the index is what the export posts back")
	assert.Equal(t, "eng · aac · Stereo", options[1].Label)
	assert.Equal(t, "jpn · aac · Stereo", options[2].Label)
}

func TestAudioTrackOptionsAreEmptyForNoTracks(t *testing.T) {
	t.Parallel()

	assert.Empty(t, AudioTrackOptions(nil))
}

// TestApplySourceFillsTheCardFromItsSource covers the fields a clip card
// takes from its probed source, and an unprobed source leaving them empty.
func TestApplySourceFillsTheCardFromItsSource(t *testing.T) {
	t.Parallel()

	item := ClipItem{ID: "c1"}

	ApplySource(&item, library.SourceInfo{
		Duration:     time.Hour,
		HDR:          true,
		AudioStreams: []library.AudioStream{{Index: 0, Language: "eng"}},
	})

	assert.True(t, item.SourceHDR)
	assert.Equal(t, time.Hour, item.MediaDuration)
	assert.Len(t, item.AudioTracks, 1)

	ApplySource(&item, library.SourceInfo{})

	assert.False(t, item.SourceHDR)
	assert.Zero(t, item.MediaDuration)
	assert.Empty(t, item.AudioTracks)
}
