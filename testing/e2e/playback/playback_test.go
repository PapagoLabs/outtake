// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

// Package playback holds the end-to-end specs for the playback panel a media
// item page embeds. The panel reports the live Plex position when the item is
// playing and an idle hint when it is not.
package playback

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Playback", func() {
	It("renders the idle panel for an item that is not playing", func(ctx SpecContext) {
		resp := testApp.Do(ctx, http.MethodGet, panelPath("e2e-idle-item"))
		body := helpers.ReadBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).To(ContainSubstring("Play this item in Plex, then mark start and end"))
	})

	It("omits the marking controls while nothing is playing", func(ctx SpecContext) {
		body := helpers.ReadBody(
			testApp.Do(ctx, http.MethodGet, panelPath("e2e-idle-item")),
		)

		Expect(body).NotTo(ContainSubstring("Set start from Plex"))
		Expect(body).NotTo(ContainSubstring("Set end from Plex"))
	})

	It("renders a panel for a Plex rating key", func(ctx SpecContext) {
		testApp.SkipWithoutPlex()

		resp := testApp.Do(ctx, http.MethodGet, panelPath("1"))
		body := helpers.ReadBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).To(ContainSubstring("Play this item in Plex, then mark start and end"))
	})
})

// panelPath builds the playback panel path for one media item.
func panelPath(mediaID string) string {
	return fmt.Sprintf("/media/item/%s/playback", mediaID)
}
