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

var _ = Describe("Plex server", func() {
	var server plex.Server

	BeforeEach(func() {
		testApp.SkipWithoutPlex()
		server = testApp.Server
	})

	Describe("Ping", func() {
		It("pings the configured server", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			Expect(client.Ping(ctx, server)).To(Succeed())
		})
	})

	Describe("GetServerIdentity", func() {
		It("reports the server identity", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			identity, err := client.GetServerIdentity(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.MachineIdentifier).NotTo(BeEmpty())
			Expect(identity.Version).NotTo(BeEmpty())
		})

		It("reports the same identity on consecutive calls", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			first, err := client.GetServerIdentity(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			second, err := client.GetServerIdentity(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			Expect(second.MachineIdentifier).To(Equal(first.MachineIdentifier))
			Expect(second.Version).To(Equal(first.Version))
		})
	})

	Describe("GetLibraries", func() {
		It("returns at least one library", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			libraries, err := client.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libraries).NotTo(BeEmpty())

			for _, library := range libraries {
				Expect(library.ID).NotTo(BeEmpty())
				Expect(library.Title).NotTo(BeEmpty())
				Expect(library.Type).NotTo(BeEmpty())
			}
		})

		It("returns a consistent count on consecutive calls", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			first, err := client.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			second, err := client.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			Expect(second).To(HaveLen(len(first)))
		})
	})

	Describe("GetMedia", func() {
		It("returns media for every library", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			libraries, err := client.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libraries).NotTo(BeEmpty())

			for _, library := range libraries {
				items, listErr := client.GetMedia(ctx, server, library.ID)
				Expect(
					listErr,
				).NotTo(HaveOccurred(), "fetch media for library %s", library.ID)
				Expect(items).NotTo(BeNil(), "media items for library %s", library.ID)

				for _, item := range items {
					Expect(item.ID).NotTo(BeEmpty())
					Expect(item.Title).NotTo(BeEmpty())
					Expect(item.Type).NotTo(BeEmpty())
					Expect(item.Duration).To(BeNumerically(">=", 0))
				}
			}
		})

		It("returns empty or an error for an unknown library", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			items, err := client.GetMedia(ctx, server, "999999999")
			if err != nil {
				Expect(err).To(HaveOccurred())

				return
			}

			Expect(items).To(BeEmpty())
		})
	})

	Describe("GetMediaPath", func() {
		It("returns an absolute path for a real item", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			first := firstMedia(ctx, client, server)
			Expect(first.ID).NotTo(BeEmpty())

			path, err := client.GetMediaPath(ctx, server, first.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(ContainSubstring("/"))
		})

		It("returns an error for an unknown numeric id", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			_, err := client.GetMediaPath(ctx, server, "999999999")
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for a malformed id", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			_, err := client.GetMediaPath(ctx, server, "invalid-id")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ValidateToken", func() {
		It("accepts the configured token", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			valid, user, err := client.ValidateToken(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
			Expect(user).NotTo(BeNil())
			Expect(user.Title).NotTo(BeEmpty())
		})
	})

	Describe("GetSessions", func() {
		It("returns the account sessions", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			sessions, err := client.GetSessions(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sessions).NotTo(BeNil())
		})
	})

	Describe("DiscoverServers", func() {
		It("discovers the servers the token reaches", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			servers, err := client.DiscoverServers(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).NotTo(BeNil())
		})
	})

	Describe("Full workflow", func() {
		It("walks identity, ping, libraries, media, and path", func(ctx SpecContext) {
			client := helpers.PlexClient(testApp.Token, helpers.PlexTVBaseURL, 30*time.Second)

			identity, err := client.GetServerIdentity(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.MachineIdentifier).NotTo(BeEmpty())

			Expect(client.Ping(ctx, server)).To(Succeed())

			libraries, err := client.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libraries).NotTo(BeEmpty())

			library := libraries[0]

			items, err := client.GetMedia(ctx, server, library.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(items).NotTo(BeEmpty())

			path, err := client.GetMediaPath(ctx, server, items[0].ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(path).NotTo(BeEmpty())

			GinkgoWriter.Printf(
				"Plex workflow: server=%s library=%s item=%s",
				identity.MachineIdentifier, library.Title, path,
			)
		})
	})
})

// firstMedia returns one media item from the first library of the server.
func firstMedia(ctx SpecContext, client *plex.Client, server plex.Server) plex.MediaItem {
	GinkgoHelper()

	libraries, err := client.GetLibraries(ctx, server)
	Expect(err).NotTo(HaveOccurred())
	Expect(libraries).NotTo(BeEmpty())

	items, err := client.GetMedia(ctx, server, libraries[0].ID)
	Expect(err).NotTo(HaveOccurred())
	Expect(items).NotTo(BeEmpty())

	return items[0]
}
