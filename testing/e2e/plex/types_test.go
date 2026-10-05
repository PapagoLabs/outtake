// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package plex

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

// These specs never reach a server, so they sit outside every gate and run
// whatever the Plex credentials are.
var _ = Describe("Plex pure functions", func() {
	DescribeTable("maps Plex types",
		func(input, expected string) {
			Expect(plex.MapPlexType(input)).To(Equal(expected))
		},
		Entry("movie", "movie", "movie"),
		Entry("show", "show", "show"),
		Entry("episode", "episode", "episode"),
		Entry("season", "season", "season"),
		Entry("album", "album", "album"),
		Entry("track", "track", "track"),
		Entry("artist", "artist", "artist"),
		Entry("photo", "photo", "photo"),
		Entry("clip", "clip", "clip"),
		Entry("unknown", "unknown", "unknown"),
		Entry("empty string", "", "unknown"),
		Entry("invalid type", "invalid_type", "unknown"),
	)

	Describe("GetAuthURL", func() {
		It("builds the plex.tv authorization URL", func() {
			authURL := anonymousClient().
				GetAuthURL("test-pin", "test-client", "http://localhost:8080/callback")

			Expect(authURL).To(ContainSubstring("app.plex.tv/auth"))
			Expect(authURL).To(ContainSubstring("clientID=test-client"))
			Expect(authURL).To(ContainSubstring("code=test-pin"))
			Expect(authURL).To(ContainSubstring(
				"context%5Bdevice%5D%5Bproduct%5D=" + helpers.Product,
			))
			Expect(authURL).To(ContainSubstring(
				"forwardUrl=http%3A%2F%2Flocalhost%3A8080%2Fcallback",
			))
		})

		It("still targets plex.tv when every parameter is empty", func() {
			authURL := anonymousClient().GetAuthURL("", "", "")

			Expect(authURL).To(ContainSubstring("app.plex.tv/auth"))
		})
	})
})

// anonymousClient returns a client that carries no token.
func anonymousClient() *plex.Client {
	return helpers.PlexClient("", helpers.PlexTVBaseURL, 30*time.Second)
}
