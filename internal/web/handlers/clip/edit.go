// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"context"
	"fmt"
	"strings"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// parseEdit reads a clip update. A JSON body may carry only the fields that
// change. A form posts every field the card shows, plus hidden copies of the
// ones it does not.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - req: The update, with nil for every field it leaves alone.
//   - err: Non-nil when the body or a form mark cannot be read.
func parseEdit(ctx fiber.Ctx) (clipdom.EditRequest, error) {
	if strings.Contains(ctx.Get(fiber.HeaderContentType), "json") {
		//nolint:wrapcheck // The error message names what the caller has to correct.
		return parseJSONEdit(ctx)
	}

	form, err := ParseRequest(ctx)
	if err != nil {
		//nolint:wrapcheck // The error message names the field and the value the user has to correct.
		return clipdom.EditRequest{}, err
	}

	return clipdom.EditRequest{
		Name:          &form.Name,
		ClipType:      &form.ClipType,
		StartTime:     &form.StartTime,
		Duration:      &form.Duration,
		Quality:       &form.Quality,
		Width:         &form.Width,
		FPS:           &form.FPS,
		AudioIndex:    &form.AudioIndex,
		CropBlackBars: &form.CropBlackBars,
		PreserveHDR:   nil,
		WebSafeColor:  nil,
	}, nil
}

// parseJSONEdit binds a JSON clip update, refusing a Keep HDR choice, which
// belongs to the profile, and negative marks.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - req: The update, with nil for every field it leaves alone.
//   - err: Non-nil when the body cannot be read, it chooses Keep HDR, or a mark
//     is negative.
func parseJSONEdit(ctx fiber.Ctx) (clipdom.EditRequest, error) {
	var req clipdom.EditRequest

	err := ctx.Bind().Body(&req)
	if err != nil {
		return clipdom.EditRequest{}, fmt.Errorf("bind json: %w: %w", api.ErrInvalidBody, err)
	}

	err = req.CheckHDRChoice()
	if err != nil {
		//nolint:wrapcheck // The error message tells the caller to choose a profile.
		return clipdom.EditRequest{}, err
	}

	err = checkMarks(valueOr(req.StartTime, 0), valueOr(req.Duration, 0))
	if err != nil {
		//nolint:wrapcheck // The error message names the mark the caller has to correct.
		return clipdom.EditRequest{}, err
	}

	return req, nil
}

// mergeEdit builds the full edit an update makes to a stored clip, taking
// the stored value for every field the update leaves out. Keep HDR is left
// for the caller, which takes it from the profile only when the edit renders.
//
// Parameters:
//   - job: The stored clip.
//   - req: The update.
//   - kind: The resolved clip type.
//   - quality: The resolved profile id.
//
// Returns:
//   - edit: The change, with every field set.
func mergeEdit(
	job *clipdom.Job,
	req clipdom.EditRequest,
	kind clipdom.Type,
	quality string,
) clipdom.Edit {
	start, length := job.StartTime, job.Duration

	if req.StartTime != nil {
		start = timecode.FromSeconds(*req.StartTime).Duration()
	}

	if req.Duration != nil {
		length = timecode.FromSeconds(*req.Duration).Duration()
	}

	return clipdom.Edit{
		Type:          kind,
		Name:          valueOr(req.Name, job.Name),
		Quality:       quality,
		Start:         start,
		Length:        length,
		Width:         valueOr(req.Width, job.Width),
		FPS:           valueOr(req.FPS, job.FPS),
		AudioIndex:    valueOr(req.AudioIndex, job.AudioIndex),
		CropBlackBars: valueOr(req.CropBlackBars, job.CropBlackBars),
		PreserveHDR:   nil,
	}
}

// resolveEditQuality resolves the profile an update names, keeping the
// stored one when it names none.
//
// Parameters:
//   - ctx: Request context.
//   - requested: The profile the update names, nil or empty for none.
//   - stored: The clip's current profile id.
//
// Returns:
//   - quality: The profile id the clip renders with.
//   - err: Non-nil when the named profile is not recognized.
func (handler *Handler) resolveEditQuality(
	ctx context.Context,
	requested *string,
	stored string,
) (string, error) {
	if requested == nil || *requested == "" {
		return stored, nil
	}

	quality, err := profile.ResolveProfile(ctx, handler.db, *requested)
	if err != nil {
		return "", fmt.Errorf("apply quality: %w", err)
	}

	return quality, nil
}

// valueOr reads an optional field against the stored value.
//
// Parameters:
//   - value: The field an update carried, nil when it left it out.
//   - stored: The value the clip already has.
//
// Returns:
//   - result: The carried value, otherwise stored.
//
//nolint:ireturn // T is the field's own type, which callers read as that type.
func valueOr[T any](value *T, stored T) T {
	if value == nil {
		return stored
	}

	return *value
}
