// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PapagoLabs/outtake/internal/clip"
)

// MaxProfileNameLen is the maximum stored clip profile name length.
const MaxProfileNameLen = 64

var (
	// errProfileName reports an empty profile name.
	errProfileName = errors.New("name is required")
	// errProfileNameLength reports a name over the length cap.
	errProfileNameLength = fmt.Errorf("name must be %d characters or fewer", MaxProfileNameLen)
	// errProfileCRF reports a CRF outside the libx264 range.
	errProfileCRF = fmt.Errorf("crf must be between %d and %d", clip.MinCRF, clip.MaxCRF)
	// errProfilePreset reports an unrecognized encoder preset.
	errProfilePreset = errors.New("unknown encoder preset")
	// errProfileAudio reports an audio bitrate out of range.
	errProfileAudio = fmt.Errorf(
		"audio bitrate must be between %d and %d kbps",
		clip.MinAudioKbps,
		clip.MaxAudioKbps,
	)
	// errProfileWidth reports a max width that is not an export size.
	errProfileWidth = errors.New("max resolution must be 720p, 1080p, 1440p, or 4K")
)

// ProfileFromFields validates raw profile form fields and builds a clip profile.
//
// Parameters:
//   - id: Profile identifier the validated fields are stored under.
//   - fields: Raw form values.
//
// Returns:
//   - profile: The validated profile, stamped with the current time.
//   - err: A validation error naming the field that failed.
func ProfileFromFields(id string, fields ProfileFields) (Profile, error) {
	// Name is required and length-capped.
	name := strings.TrimSpace(fields.Name)
	if name == "" {
		return Profile{}, errProfileName
	}

	if len(name) > MaxProfileNameLen {
		return Profile{}, errProfileNameLength
	}

	crf, ok := profileInt(fields.CRF, clip.ValidCRF)
	if !ok {
		return Profile{}, errProfileCRF
	}

	if !clip.ValidEncoderPreset(fields.Preset) {
		return Profile{}, errProfilePreset
	}

	audioKbps, ok := profileInt(fields.AudioKbps, clip.ValidAudioKbps)
	if !ok {
		return Profile{}, errProfileAudio
	}

	maxWidth, ok := profileInt(fields.MaxWidth, clip.ValidOutputWidth)
	if !ok {
		return Profile{}, errProfileWidth
	}

	now := time.Now().UTC()

	return Profile{
		ID:        id,
		Name:      name,
		CRF:       crf,
		Preset:    fields.Preset,
		AudioKbps: audioKbps,
		MaxWidth:  maxWidth,
		IsDefault: fields.IsDefault,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// profileInt parses a posted integer and checks it against valid.
//
// Parameters:
//   - raw: Posted form value.
//   - valid: Domain range check for the parsed value.
//
// Returns:
//   - value: The parsed integer, zero when raw is not an integer.
//   - ok: False when raw is not an integer or fails the range check.
func profileInt(raw string, valid func(int) bool) (int, bool) {
	value, err := strconv.Atoi(raw)
	if err != nil || !valid(value) {
		return 0, false
	}

	return value, true
}
