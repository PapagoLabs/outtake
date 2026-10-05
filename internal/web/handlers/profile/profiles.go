// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"fmt"
	"io"

	fiber "github.com/gofiber/fiber/v3"

	clipprofile "github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// Handler serves the clip profile settings page and the edits posted to it.
type Handler struct {
	profiles *clipprofile.Service
}

// New returns a clip profile handler wired to the supplied collaborators.
//
// Parameters:
//   - store: Clip profile service the edits are applied through.
//
// Returns:
//   - handler: A ready-to-use clip profile handler.
func New(store *clipprofile.Service) *Handler {
	return &Handler{profiles: store}
}

// ClipProfiles renders the clip profile settings page.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - err: Render or redirect error, or nil on success.
func (handler *Handler) ClipProfiles(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return pages.ClipProfiles(pages.ClipProfilesProps{
			Profiles: view.ClipProfileItems(handler.profiles.List(ctx.Context())),
			Presets:  clipprofile.EncoderPresets(),
			Widths:   clipprofile.OutputWidths(),
			Error:    ctx.Query(routes.QueryError),
		}).Render(ctx.Context(), writer)
	})
}

// CreateClipProfile stores a new clip profile.
//
// Parameters:
//   - ctx: Request context carrying the profile form fields.
//
// Returns:
//   - err: Redirect error, or nil on success.
func (handler *Handler) CreateClipProfile(ctx fiber.Ctx) error {
	_, err := handler.profiles.Create(ctx.Context(), profileFields(ctx))

	//nolint:wrapcheck // The helper wraps its own failure with what it was doing.
	return redirectToProfiles(ctx, err)
}

// DeleteClipProfile removes a profile.
//
// Parameters:
//   - ctx: Request context carrying the profile id route parameter.
//
// Returns:
//   - err: Redirect error, or nil on success.
func (handler *Handler) DeleteClipProfile(ctx fiber.Ctx) error {
	//nolint:wrapcheck // The helper wraps its own failure with what it was doing.
	return redirectToProfiles(
		ctx,
		handler.profiles.Delete(ctx.Context(), ctx.Params(routes.ParamID)),
	)
}

// SetDefaultClipProfile marks a profile as the default.
//
// Parameters:
//   - ctx: Request context carrying the profile id route parameter.
//
// Returns:
//   - err: Redirect error, or nil on success.
func (handler *Handler) SetDefaultClipProfile(ctx fiber.Ctx) error {
	//nolint:wrapcheck // The helper wraps its own failure with what it was doing.
	return redirectToProfiles(
		ctx,
		handler.profiles.SetDefault(ctx.Context(), ctx.Params(routes.ParamID)),
	)
}

// UpdateClipProfile saves edits to an existing profile.
//
// Parameters:
//   - ctx: Request context carrying the profile id and form fields.
//
// Returns:
//   - err: Redirect error, or nil on success.
func (handler *Handler) UpdateClipProfile(ctx fiber.Ctx) error {
	_, err := handler.profiles.Update(
		ctx.Context(),
		ctx.Params(routes.ParamID),
		profileFields(ctx),
	)

	//nolint:wrapcheck // The helper wraps its own failure with what it was doing.
	return redirectToProfiles(ctx, err)
}

// profileFields reads the clip profile form values as posted.
//
// Parameters:
//   - ctx: Request context carrying the profile form fields.
//
// Returns:
//   - fields: The raw form values, which the service validates.
func profileFields(ctx fiber.Ctx) clipprofile.ProfileFields {
	return clipprofile.ProfileFields{
		Name:      ctx.FormValue("name"),
		CRF:       ctx.FormValue("crf"),
		Preset:    ctx.FormValue("preset"),
		AudioKbps: ctx.FormValue("audioKbps"),
		MaxWidth:  ctx.FormValue("maxWidth"),
		IsDefault: routes.IsFormChecked(ctx.FormValue("isDefault")),
	}
}

// redirectToProfiles answers a profile edit, carrying any failure to the page.
//
// Parameters:
//   - ctx: Request context.
//   - err: Failure from the profile service, which may be nil.
//
// Returns:
//   - err: The redirect result.
func redirectToProfiles(ctx fiber.Ctx, err error) error {
	target := routes.PathSettingsProfiles
	if err != nil {
		target = respond.PathWithError(target, err.Error())
	}

	redirectErr := respond.RedirectTo(ctx, target)
	if redirectErr != nil {
		return fmt.Errorf("redirect to profiles: %w", redirectErr)
	}

	return nil
}
