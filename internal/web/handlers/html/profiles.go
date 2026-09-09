// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package html

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"uuid"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

const (
	// MaxProfileNameLen is the maximum stored clip profile name length.
	maxProfileNameLen = 64
)

var (
	// ErrProfileName is returned when a profile name is empty.
	errProfileName = errors.New("name is required")
	// ErrProfileNameLength is returned when a profile name is too long.
	errProfileNameLength = fmt.Errorf("name must be %d characters or fewer", maxProfileNameLen)
	// ErrProfileCRF is returned when CRF is outside the libx264 range.
	errProfileCRF = fmt.Errorf("crf must be between %d and %d", media.MinCRF, media.MaxCRF)
	// ErrProfilePreset is returned when the encoder preset is not recognized.
	errProfilePreset = errors.New("unknown encoder preset")
	// ErrProfileAudio is returned when audio bitrate is out of range.
	errProfileAudio = fmt.Errorf(
		"audio bitrate must be between %d and %d kbps",
		media.MinAudioKbps,
		media.MaxAudioKbps,
	)
	// ErrProfileWidth is returned when max width is not a supported export size.
	errProfileWidth = errors.New("max resolution must be 720p, 1080p, 1440p, or 4K")
)

// ClipProfiles renders the clip profile settings page.
func (handler *HTMLHandler) ClipProfiles(ctx fiber.Ctx) error {
	return shared.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.ClipProfiles(pages.ClipProfilesProps{
			Profiles: toClipProfileItems(handler.storedClipProfiles(ctx)),
			Presets:  media.EncoderPresets,
			Widths:   outputWidthOptions(),
			Error:    ctx.Query(shared.QueryError),
		}).Render(ctx.Context(), writer)
	})
}

// CreateClipProfile stores a new clip profile.
func (handler *HTMLHandler) CreateClipProfile(ctx fiber.Ctx) error {
	profile, err := parseClipProfileForm(ctx, uuid.New().String())
	if err != nil {
		return shared.RedirectTo(ctx, shared.PathWithError(shared.PathSettingsProfiles, err.Error()))
	}

	err = handler.db.SaveClipProfile(ctx.Context(), profile)
	if err != nil {
		return shared.RedirectTo(ctx, shared.PathWithError(shared.PathSettingsProfiles, err.Error()))
	}

	return shared.RedirectTo(ctx, shared.PathSettingsProfiles)
}

// UpdateClipProfile saves edits to an existing profile.
func (handler *HTMLHandler) UpdateClipProfile(ctx fiber.Ctx) error {
	existing, err := handler.db.GetClipProfile(ctx.Context(), ctx.Params(paramID))
	if err != nil {
		return shared.RedirectTo(ctx, shared.PathWithError(shared.PathSettingsProfiles, err.Error()))
	}

	profile, err := parseClipProfileForm(ctx, existing.ID)
	if err != nil {
		return shared.RedirectTo(ctx, shared.PathWithError(shared.PathSettingsProfiles, err.Error()))
	}

	profile.CreatedAt = existing.CreatedAt
	profile.IsDefault = existing.IsDefault

	err = handler.db.SaveClipProfile(ctx.Context(), profile)
	if err != nil {
		return shared.RedirectTo(ctx, shared.PathWithError(shared.PathSettingsProfiles, err.Error()))
	}

	return shared.RedirectTo(ctx, shared.PathSettingsProfiles)
}

// DeleteClipProfile removes a profile.
func (handler *HTMLHandler) DeleteClipProfile(ctx fiber.Ctx) error {
	err := handler.db.DeleteClipProfile(ctx.Context(), ctx.Params(paramID))
	if err != nil {
		return shared.RedirectTo(ctx, shared.PathWithError(shared.PathSettingsProfiles, err.Error()))
	}

	return shared.RedirectTo(ctx, shared.PathSettingsProfiles)
}

// SetDefaultClipProfile marks a profile as the default.
func (handler *HTMLHandler) SetDefaultClipProfile(ctx fiber.Ctx) error {
	err := handler.db.SetDefaultClipProfile(ctx.Context(), ctx.Params(paramID))
	if err != nil {
		return shared.RedirectTo(ctx, shared.PathWithError(shared.PathSettingsProfiles, err.Error()))
	}

	return shared.RedirectTo(ctx, shared.PathSettingsProfiles)
}

// storedClipProfiles loads stored profiles for HTML pages.
func (handler *HTMLHandler) storedClipProfiles(ctx fiber.Ctx) []database.ClipProfile {
	profiles, err := handler.db.ListClipProfiles(ctx.Context())
	if err != nil {
		return nil
	}

	return profiles
}

// clipProfileOptions maps stored profiles onto form select options.
func (handler *HTMLHandler) clipProfileOptions(ctx fiber.Ctx) []view.ClipProfileOption {
	profiles := handler.storedClipProfiles(ctx)
	options := make([]view.ClipProfileOption, 0, len(profiles))

	for i := range profiles {
		profile := profiles[i]

		options = append(options, view.ClipProfileOption{
			ID:        profile.ID,
			Name:      profile.Name,
			IsDefault: profile.IsDefault,
		})
	}

	if len(options) == 0 {
		return builtinProfileOptions()
	}

	return options
}

// builtinProfileOptions is used when the profile table cannot be read.
func builtinProfileOptions() []view.ClipProfileOption {
	return []view.ClipProfileOption{
		{ID: string(media.ClipQualityLow), Name: "Low", IsDefault: false},
		{ID: string(media.ClipQualityMedium), Name: "Medium", IsDefault: true},
		{ID: string(media.ClipQualityHigh), Name: "High", IsDefault: false},
	}
}

// parseClipProfileForm binds and validates profile fields from a form post.
func parseClipProfileForm(ctx fiber.Ctx, id string) (database.ClipProfile, error) {
	profile, err := clipProfileFromFields(
		id,
		ctx.FormValue("name"),
		ctx.FormValue("crf"),
		ctx.FormValue("preset"),
		ctx.FormValue("audioKbps"),
		ctx.FormValue("maxWidth"),
	)
	if err != nil {
		return database.ClipProfile{}, fmt.Errorf("parse clip profile: %w", err)
	}

	profile.IsDefault = ctx.FormValue("isDefault") == "1"

	return profile, nil
}

// clipProfileFromFields validates and builds a clip profile.
func clipProfileFromFields(
	id, name, crfRaw, preset, audioRaw, widthRaw string,
) (database.ClipProfile, error) {
	// Name is required and length-capped.
	name = strings.TrimSpace(name)
	if name == "" {
		return database.ClipProfile{}, errProfileName
	}

	if len(name) > maxProfileNameLen {
		return database.ClipProfile{}, errProfileNameLength
	}

	crf, ok := parseProfileInt(crfRaw, media.ValidCRF)
	if !ok {
		return database.ClipProfile{}, errProfileCRF
	}

	if !media.ValidEncoderPreset(preset) {
		return database.ClipProfile{}, errProfilePreset
	}

	audioKbps, ok := parseProfileInt(audioRaw, media.ValidAudioKbps)
	if !ok {
		return database.ClipProfile{}, errProfileAudio
	}

	maxWidth, ok := parseProfileInt(widthRaw, media.ValidOutputWidth)
	if !ok {
		return database.ClipProfile{}, errProfileWidth
	}

	now := time.Now().UTC()

	return database.ClipProfile{
		ID:        id,
		Name:      name,
		CRF:       crf,
		Preset:    preset,
		AudioKbps: audioKbps,
		MaxWidth:  maxWidth,
		IsDefault: false,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// parseProfileInt parses a form integer and checks it with valid.
func parseProfileInt(raw string, valid func(int) bool) (int, bool) {
	value, err := strconv.Atoi(raw)
	if err != nil || !valid(value) {
		return 0, false
	}

	return value, true
}

// toClipProfileItems maps stored profiles onto page view models.
func toClipProfileItems(profiles []database.ClipProfile) []pages.ClipProfileItem {
	items := make([]pages.ClipProfileItem, 0, len(profiles))

	for i := range profiles {
		profile := profiles[i]

		items = append(items, pages.ClipProfileItem{
			ID:        profile.ID,
			Name:      profile.Name,
			CRF:       profile.CRF,
			Preset:    profile.Preset,
			AudioKbps: profile.AudioKbps,
			MaxWidth:  profile.MaxWidth,
			IsDefault: profile.IsDefault,
		})
	}

	return items
}

// outputWidthOptions lists selectable clip export widths.
func outputWidthOptions() []pages.OutputWidthOption {
	options := make([]pages.OutputWidthOption, 0, len(media.OutputWidths))

	for _, width := range media.OutputWidths {
		options = append(options, pages.OutputWidthOption{
			Width: width,
			Label: media.OutputWidthLabel(width),
		})
	}

	return options
}
