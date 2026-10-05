// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package home

import (
	"io"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/timecode"
	"github.com/PapagoLabs/outtake/internal/web/components/playback"
	"github.com/PapagoLabs/outtake/internal/web/respond"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// Playback renders live Plex playback for a media item.
//
// Parameters:
//   - ctx: Request carrying the media item id.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *Handler) Playback(ctx fiber.Ctx) error {
	mediaID := ctx.Params(routes.ParamID)
	props := view.Playback{
		Playing:    false,
		ViewOffset: 0,
		Title:      "",
	}

	sessions := handler.auth.Sessions()
	for index := range sessions {
		if sessions[index].MediaItem.ID != mediaID {
			continue
		}

		props.Playing = true
		props.ViewOffset = timecode.FromSeconds(sessions[index].ViewOffset).Duration()
		props.Title = sessions[index].Title

		break
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return playback.PlaybackPanel(props).Render(ctx.Context(), writer)
	})
}
