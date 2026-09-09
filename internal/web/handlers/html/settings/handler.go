// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package settings

import (
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"uuid"

	mediaquality "github.com/PapagoLabs/outtake/internal/media/quality"

	htmldeps "github.com/PapagoLabs/outtake/internal/web/handlers/html/deps"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	"github.com/PapagoLabs/outtake/internal/web/pages/settings"
)

// Handler serves settings HTML routes.
type Handler struct {
	rt *htmldeps.Runtime
}

// New constructs a settings HTML handler.
//
// Parameters:
//   - rt: Rt.
//
// Returns:
//   - handler: A settings HTML handler.
func New(rt *htmldeps.Runtime) *Handler {
	return &Handler{rt: rt}
}

// ClipProfiles renders the clip profiles settings page.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) ClipProfiles(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return settings.ClipProfiles(settings.ClipProfilesProps{
			Profiles: htmldeps.ToClipProfileItems(h.rt.StoredClipProfiles(ctx)),
			Presets:  mediaquality.EncoderPresets,
			Widths:   htmldeps.OutputWidthOptions(),
			Error:    ctx.Query(respond.QueryError),
		}).Render(ctx.Context(), writer)
	})
}

// CreateClipProfile creates a clip profile from the submitted form.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) CreateClipProfile(ctx fiber.Ctx) error {
	profile, err := htmldeps.ParseClipProfileForm(ctx, uuid.New().String())
	if err != nil {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathSettingsProfiles, err.Error()))
	}

	err = h.rt.DB.SaveClipProfile(ctx.Context(), profile)
	if err != nil {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathSettingsProfiles, err.Error()))
	}

	return respond.RedirectTo(ctx, respond.PathSettingsProfiles)
}

// UpdateClipProfile updates a clip profile from the submitted form.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) UpdateClipProfile(ctx fiber.Ctx) error {
	existing, err := h.rt.DB.GetClipProfile(ctx.Context(), ctx.Params(htmldeps.ParamID))
	if err != nil {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathSettingsProfiles, err.Error()))
	}

	profile, err := htmldeps.ParseClipProfileForm(ctx, existing.ID)
	if err != nil {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathSettingsProfiles, err.Error()))
	}

	profile.CreatedAt = existing.CreatedAt
	profile.IsDefault = existing.IsDefault

	err = h.rt.DB.SaveClipProfile(ctx.Context(), profile)
	if err != nil {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathSettingsProfiles, err.Error()))
	}

	return respond.RedirectTo(ctx, respond.PathSettingsProfiles)
}

// DeleteClipProfile deletes a clip profile.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) DeleteClipProfile(ctx fiber.Ctx) error {
	err := h.rt.DB.DeleteClipProfile(ctx.Context(), ctx.Params(htmldeps.ParamID))
	if err != nil {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathSettingsProfiles, err.Error()))
	}

	return respond.RedirectTo(ctx, respond.PathSettingsProfiles)
}

// SetDefaultClipProfile marks a clip profile as the default.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (h *Handler) SetDefaultClipProfile(ctx fiber.Ctx) error {
	err := h.rt.DB.SetDefaultClipProfile(ctx.Context(), ctx.Params(htmldeps.ParamID))
	if err != nil {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathSettingsProfiles, err.Error()))
	}

	return respond.RedirectTo(ctx, respond.PathSettingsProfiles)
}

// Appearance renders the appearance settings page.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - err: The error, if any.
func (*Handler) Appearance(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return settings.Appearance().Render(ctx.Context(), writer)
	})
}
