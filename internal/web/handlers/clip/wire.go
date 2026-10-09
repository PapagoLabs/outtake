// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package clip

import (
	"fmt"
	"strings"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// ReturnPath sends form posts back to the source media item when possible.
//
// Parameters:
//   - mediaID: Plex media id from the request, which may be empty.
//
// Returns:
//   - path: The media item page when mediaID is set, otherwise routes.PathClips.
func ReturnPath(mediaID string) string {
	if mediaID != "" {
		return routes.ItemURL(mediaID, nil)
	}

	return routes.PathClips
}

// RequestEdit translates a parsed request into the edit it describes, for a
// clip of the given type.
//
// Parameters:
//   - req: Parsed request carrying the marks and the encoding options.
//   - kind: Normalized clip type.
//
// Returns:
//   - edit: The change the request describes.
func RequestEdit(req api.ClipRequest, kind clipdom.Type) clipdom.Edit {
	edit := clipEdit(req)

	edit.Type = kind

	return edit
}

// clipEdit translates a parsed request into the edit it describes.
//
// Parameters:
//   - req: Parsed request carrying the marks and the encoding options.
//
// Returns:
//   - edit: The change the request asks a stored clip to make.
func clipEdit(req api.ClipRequest) clipdom.Edit {
	start, duration := requestWindow(req)

	return clipdom.Edit{
		Name:          req.Name,
		Quality:       req.Quality,
		Start:         start,
		Length:        duration,
		Width:         req.Width,
		FPS:           req.FPS,
		AudioIndex:    req.AudioIndex,
		CropBlackBars: req.CropBlackBars,
		PreserveHDR:   nil,
	}
}

// requestWindow reads a request's marks as the durations a clip carries.
//
// Parameters:
//   - req: Parsed request carrying the marks.
//
// Returns:
//   - start: Start offset as a duration.
//   - duration: Selection length as a duration.
//
//nolint:nonamedreturns // the names are what tell the two same-typed results apart.
func requestWindow(req api.ClipRequest) (start, duration time.Duration) {
	start = timecode.FromSeconds(req.StartTime).Duration()
	duration = timecode.FromSeconds(req.Duration).Duration()

	return start, duration
}

// parseJSONRequest binds a JSON clip request, refusing negative marks and a
// Keep HDR choice, which belongs to the profile.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - req: The bound request.
//   - err: Non-nil when the body cannot be read, a mark is negative, or the
//     request chooses Keep HDR.
func parseJSONRequest(ctx fiber.Ctx) (api.ClipRequest, error) {
	var req api.ClipRequest

	err := ctx.Bind().Body(&req)
	if err != nil {
		return api.ClipRequest{}, fmt.Errorf("bind json: %w", err)
	}

	err = req.CheckHDRChoice()
	if err != nil {
		//nolint:wrapcheck // The error message tells the caller to choose a profile.
		return api.ClipRequest{}, err
	}

	err = checkMarks(req.StartTime, req.Duration)
	if err != nil {
		//nolint:wrapcheck // The error message names the mark the caller has to correct.
		return api.ClipRequest{}, err
	}

	return req, nil
}

// checkMarks refuses marks a JSON caller sent below zero. Converting one to a
// duration reads it as zero, which would quietly move the clip.
//
// Parameters:
//   - start: Start mark in seconds.
//   - length: Selection length in seconds.
//
// Returns:
//   - err: Non-nil when either mark is negative.
func checkMarks(start, length float64) error {
	if start < 0 {
		return fmt.Errorf("%w: the start must not be negative", clipdom.ErrRangeOutsideMedia)
	}

	if length < 0 {
		return fmt.Errorf("%w: the length must not be negative", clipdom.ErrInvalidDuration)
	}

	return nil
}

// formDuration parses a timecode form field as a duration.
//
// Parameters:
//   - ctx: Request context.
//   - name: Form field holding the timecode.
//   - label: How the field is named to the user.
//
// Returns:
//   - duration: The parsed duration, zero when the field is empty.
//   - err: Non-nil when the field holds something that is not a timecode.
func formDuration(ctx fiber.Ctx, name, label string) (time.Duration, error) {
	value := ctx.FormValue(name)

	spacesOnly := value != "" && strings.TrimSpace(value) == ""

	tc, err := timecode.Parse(value)
	if err != nil || spacesOnly {
		return 0, fmt.Errorf(
			"%w: the %s must be a timecode such as 00:01:23.456, not %q",
			timecode.ErrInvalidTimecode, label, value,
		)
	}

	return tc.Duration(), nil
}

// ParseRequest binds JSON or form fields into a clip request.
//
// Parameters:
//   - ctx: Request context.
//
// Returns:
//   - req: The bound request.
//   - err: Non-nil when the body or a mark field cannot be read.
func ParseRequest(ctx fiber.Ctx) (api.ClipRequest, error) {
	if strings.Contains(ctx.Get(fiber.HeaderContentType), "json") {
		//nolint:wrapcheck // The error message names what the caller has to correct.
		return parseJSONRequest(ctx)
	}

	start, err := formDuration(ctx, "startTime", "start")
	if err != nil {
		//nolint:wrapcheck // The error message names the field and the value the user has to correct.
		return api.ClipRequest{}, err
	}

	end, err := formDuration(ctx, "endTime", "end")
	if err != nil {
		//nolint:wrapcheck // The error message names the field and the value the user has to correct.
		return api.ClipRequest{}, err
	}

	var duration float64

	if end > start {
		duration = (end - start).Seconds()
	}

	return api.ClipRequest{
		Name:          formString(ctx, "name"),
		MediaID:       formString(ctx, "mediaId"),
		MediaTitle:    formString(ctx, "mediaTitle"),
		MediaType:     formString(ctx, "mediaType"),
		StartTime:     start.Seconds(),
		Duration:      duration,
		Quality:       formString(ctx, "quality"),
		ClipType:      formString(ctx, "clipType"),
		Width:         respond.FormInt(ctx, "width"),
		FPS:           respond.FormInt(ctx, "fps"),
		AudioIndex:    respond.FormInt(ctx, "audioIndex"),
		CropBlackBars: routes.IsFormChecked(ctx.FormValue("cropBlackBars")),
		PreserveHDR:   nil,
		WebSafeColor:  nil,
	}, nil
}

// formString reads one form field as a string the caller may keep.
//
// Parameters:
//   - ctx: Request context.
//   - name: Form field to read.
//
// Returns:
//   - value: The field value, copied out of the request buffer.
func formString(ctx fiber.Ctx, name string) string {
	return strings.Clone(ctx.FormValue(name))
}
