// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package plex

import (
	"net/url"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Plex errors", func() {
	Describe("Unreachable server", func() {
		It("fails to ping", func(ctx SpecContext) {
			client := helpers.PlexClient("invalid-token-12345", "http://192.0.2.1:32400", 5*time.Second)

			Expect(client.Ping(ctx, unreachableServer())).To(HaveOccurred())
		})

		It("fails to list libraries", func(ctx SpecContext) {
			client := helpers.PlexClient("invalid-token-12345", "http://192.0.2.1:32400", 5*time.Second)

			_, err := client.GetLibraries(ctx, unreachableServer())
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Invalid token", func() {
		It("rejects a token no account holds", func(ctx SpecContext) {
			client := helpers.PlexClient("invalid-token-12345", helpers.PlexTVBaseURL, 30*time.Second)

			valid, _, err := client.ValidateToken(ctx)
			Expect(err).To(HaveOccurred())
			Expect(valid).To(BeFalse())
		})

		It("fails to list libraries on the configured server", func(ctx SpecContext) {
			testApp.SkipWithoutPlex()

			serverURL := os.Getenv(helpers.ServerURLEnv)

			parsed, err := url.Parse(serverURL)
			Expect(err).NotTo(HaveOccurred())

			client := helpers.PlexClient("invalid-token-12345", serverURL, 30*time.Second)

			_, err = client.GetLibraries(ctx, plex.Server{
				Name:    "Invalid Token Server",
				Address: parsed.Hostname(),
				Port:    32400,
				Token:   "invalid-token-12345",
				Scheme:  parsed.Scheme,
			})
			Expect(err).To(HaveOccurred())
		})
	})
})

// unreachableServer describes a server nothing answers on.
func unreachableServer() plex.Server {
	return plex.Server{
		Name:    "Unreachable Server",
		Address: "192.0.2.1",
		Port:    32400,
		Token:   "invalid-token-12345",
		Scheme:  "http",
	}
}
