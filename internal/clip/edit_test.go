// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditValidateRejectsAGIFOutsideTheEncoderBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		width   int
		fps     int
		wantErr error
	}{
		{name: "the bounds themselves are accepted", width: MinGIFWidth, fps: MinGIFFPS},
		{name: "the upper bounds are accepted", width: MaxGIFWidth, fps: MaxGIFFPS},
		{name: "an unset width and frame rate are accepted"},
		{name: "a width under the minimum", width: MinGIFWidth - 1, wantErr: ErrInvalidGIFWidth},
		{name: "a width over the maximum", width: MaxGIFWidth + 1, wantErr: ErrInvalidGIFWidth},
		{name: "a frame rate under the minimum", fps: MinGIFFPS - 1, wantErr: ErrInvalidGIFFPS},
		{name: "a frame rate over the maximum", fps: MaxGIFFPS + 1, wantErr: ErrInvalidGIFFPS},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			edit := Edit{Length: 5 * time.Second, Width: test.width, FPS: test.fps}

			err := edit.Validate(TypeGIF, Source{Length: 2 * time.Hour}, 10*time.Minute)

			if test.wantErr == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestEditValidateIgnoresGIFBoundsForEveryOtherType(t *testing.T) {
	t.Parallel()

	edit := Edit{
		Length: 5 * time.Second,
		Width:  MaxGIFWidth + 1000,
		FPS:    MaxGIFFPS + 100,
	}

	require.NoError(t, edit.Validate(TypeClip, Source{Length: 2 * time.Hour}, 10*time.Minute))
	require.NoError(t, edit.Validate(TypeScreenshot, Source{Length: 2 * time.Hour}, 10*time.Minute))
}

func TestEditValidateReportsWhichBoundItBroke(t *testing.T) {
	t.Parallel()

	tooLong := Edit{Start: 30 * time.Second, Length: 10*time.Minute + time.Second}
	require.ErrorIs(
		t,
		tooLong.Validate(TypeClip, Source{Length: 2 * time.Hour}, 10*time.Minute),
		ErrInvalidDuration,
	)
	assert.Contains(
		t,
		tooLong.Validate(TypeClip, Source{Length: 2 * time.Hour}, 10*time.Minute).Error(),
		"validate duration",
		"the message names the check that failed",
	)

	outOfRange := Edit{Start: 11 * time.Hour, Length: 20 * time.Second}
	require.ErrorIs(
		t,
		outOfRange.Validate(TypeClip, Source{Length: 2 * time.Hour}, 10*time.Minute),
		ErrRangeOutsideMedia,
	)
	assert.Contains(
		t,
		outOfRange.Validate(TypeClip, Source{Length: 2 * time.Hour}, 10*time.Minute).Error(),
		"check range",
		"the message names the check that failed",
	)
}

func TestClipApplyWritesEveryEditableField(t *testing.T) {
	t.Parallel()

	stored := &Clip{
		ID:            "gif-1",
		Type:          TypeGIF,
		Name:          "Old name",
		Quality:       "archive",
		StartTime:     10 * time.Second,
		Duration:      15 * time.Second,
		Width:         1280,
		FPS:           24,
		AudioIndex:    1,
		CropBlackBars: false,
	}

	stamp := stored.UpdatedAt

	stored.Apply(Edit{
		Type:          TypeClip,
		Name:          "New name",
		Quality:       string(ClipQualityMedium),
		Start:         42345 * time.Millisecond,
		Length:        8 * time.Second,
		Width:         720,
		FPS:           12,
		AudioIndex:    3,
		CropBlackBars: true,
		WebSafeColor:  new(true),
	})

	assert.Equal(t, TypeClip, stored.Type)
	assert.Equal(t, "New name", stored.Name)
	assert.Equal(t, string(ClipQualityMedium), stored.Quality)
	assert.Equal(t, 42345*time.Millisecond, stored.StartTime)
	assert.Equal(t, 8*time.Second, stored.Duration)
	assert.Equal(t, 720, stored.Width)
	assert.Equal(t, 12, stored.FPS)
	assert.Equal(t, 3, stored.AudioIndex)
	assert.True(t, stored.CropBlackBars)
	assert.True(t, stored.WebSafeColor)
	assert.False(t, stamp.IsZero() && stored.UpdatedAt.IsZero(),
		"an applied edit stamps the clip as changed")
}

func TestClipApplyKeepsOmittedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		stored    Clip
		edit      Edit
		wantType  Type
		wantName  string
		wantQuali string
	}{
		{
			name:     "an empty edit keeps everything it does not name",
			stored:   Clip{Type: TypeClip, Name: "Old name", Quality: "archive"},
			edit:     Edit{},
			wantType: TypeClip, wantName: "Old name", wantQuali: "archive",
		},
		{
			name:     "an edit that names only a type keeps the stored name and profile",
			stored:   Clip{Type: TypeClip, Name: "Old name", Quality: "archive"},
			edit:     Edit{Type: TypeGIF},
			wantType: TypeGIF, wantName: "Old name", wantQuali: "archive",
		},
		{
			name:     "an edit that names only a profile keeps the stored type and name",
			stored:   Clip{Type: TypeClip, Name: "Old name", Quality: "archive"},
			edit:     Edit{Quality: string(ClipQualityMedium)},
			wantType: TypeClip, wantName: "Old name", wantQuali: string(ClipQualityMedium),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stored := test.stored

			stored.Apply(test.edit)

			assert.Equal(t, test.wantType, stored.Type)
			assert.Equal(t, test.wantName, stored.Name)
			assert.Equal(t, test.wantQuali, stored.Quality)
		})
	}
}

func TestClipApplyPreservesAnOmittedWebSafeColor(t *testing.T) {
	t.Parallel()

	stored := &Clip{WebSafeColor: true}

	stored.Apply(Edit{Start: time.Second, Length: 5 * time.Second})
	assert.True(t, stored.WebSafeColor, "an edit that carried no setting leaves the stored one")

	stored.Apply(Edit{WebSafeColor: new(false)})
	assert.False(t, stored.WebSafeColor, "an explicit off is honored against a stored on")
}

func TestClipApplyPreservesAnOmittedKeepHDR(t *testing.T) {
	t.Parallel()

	stored := &Clip{PreserveHDR: true}

	stored.Apply(Edit{Start: time.Second, Length: 5 * time.Second})
	assert.True(t, stored.PreserveHDR, "an edit that carried no setting leaves the stored one")

	stored.Apply(Edit{PreserveHDR: new(false)})
	assert.False(t, stored.PreserveHDR, "an explicit off is honored against a stored on")
}

// TestRendersLike covers which fields decide the rendered file: a name or a
// listing field leaves it the same, and every render field changes it.
func TestRendersLike(t *testing.T) {
	t.Parallel()

	base := Clip{
		ID: "x", Type: TypeClip, Name: "Intro", MediaID: "42", MediaTitle: "Movie",
		StartTime: time.Second, Duration: 5 * time.Second, Quality: "medium",
	}

	tests := []struct {
		name   string
		change func(*Clip)
		same   bool
	}{
		{"a new name", func(c *Clip) { c.Name = "Outro" }, true},
		{"a new title", func(c *Clip) { c.MediaTitle = "Film" }, true},
		{"a new update time", func(c *Clip) { c.UpdatedAt = time.Now() }, true},
		{"a new type", func(c *Clip) { c.Type = TypeGIF }, false},
		{"a new start", func(c *Clip) { c.StartTime = 2 * time.Second }, false},
		{"a new length", func(c *Clip) { c.Duration = time.Second }, false},
		{"a new profile", func(c *Clip) { c.Quality = "high" }, false},
		{"a new width", func(c *Clip) { c.Width = 480 }, false},
		{"a new frame rate", func(c *Clip) { c.FPS = 12 }, false},
		{"a new audio track", func(c *Clip) { c.AudioIndex = 1 }, false},
		{"black bars trimmed", func(c *Clip) { c.CropBlackBars = true }, false},
		{"web-safe color", func(c *Clip) { c.WebSafeColor = true }, false},
		{"HDR kept", func(c *Clip) { c.PreserveHDR = true }, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			changed := base
			test.change(&changed)

			assert.Equal(t, test.same, changed.RendersLike(&base))
		})
	}
}

// TestEditValidateChecksTheAudioTrack covers the audio track bound: a track
// the source carries passes, and one it does not is refused rather than
// rendering a silent clip.
func TestEditValidateChecksTheAudioTrack(t *testing.T) {
	t.Parallel()

	twoTracks := Source{Length: 2 * time.Hour, AudioTracks: 2, Probed: true}
	silent := Source{Length: 2 * time.Hour, AudioTracks: 0, Probed: true}
	unprobed := Source{}

	tests := []struct {
		name   string
		kind   Type
		index  int
		source Source
		valid  bool
	}{
		{"the first of two tracks", TypeClip, 0, twoTracks, true},
		{"the second of two tracks", TypeClip, 1, twoTracks, true},
		{"a third of two tracks", TypeClip, 2, twoTracks, false},
		{"a negative track", TypeClip, -1, twoTracks, false},
		{"the default track of a silent source", TypeClip, 0, silent, true},
		{"a second track of a silent source", TypeClip, 1, silent, false},
		{"any track of a source that could not be probed", TypeClip, 4, unprobed, true},
		{"a GIF, which carries no audio", TypeGIF, 4, twoTracks, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			edit := Edit{Start: time.Second, Length: 5 * time.Second, AudioIndex: test.index}

			err := edit.Validate(test.kind, test.source, 10*time.Minute)
			if test.valid {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, ErrNoSuchAudioTrack)
		})
	}
}
