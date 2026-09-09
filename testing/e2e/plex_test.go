// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package e2e

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/plex"
)

const testToken = "test-token"

const plexBaseURL = "https://plex.tv"

var _ = Describe("Plex", func() {
	var server plex.Server

	BeforeEach(func() {
		server = getPlexServerConfig()
	})

	Describe("GetLibraries", func() {
		It("returns at least one library", func(ctx SpecContext) {
			c := newPlexClient(server)

			libs, err := c.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libs).NotTo(BeEmpty())

			for _, lib := range libs {
				Expect(lib.ID).NotTo(BeEmpty())
				Expect(lib.Title).NotTo(BeEmpty())
				Expect(lib.Type).NotTo(BeEmpty())
			}
		})

		It("returns consistent count on consecutive calls", func(ctx SpecContext) {
			c := newPlexClient(server)

			libs1, err := c.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			libs2, err := c.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			Expect(libs2).To(HaveLen(len(libs1)))
		})
	})

	Describe("GetServerIdentity", func() {
		It("returns server identity", func(ctx SpecContext) {
			c := newPlexClient(server)

			identity, err := c.GetServerIdentity(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.MachineIdentifier).NotTo(BeEmpty())
			Expect(identity.Version).NotTo(BeEmpty())
		})

		It("returns consistent identity on consecutive calls", func(ctx SpecContext) {
			c := newPlexClient(server)

			identity1, err := c.GetServerIdentity(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			identity2, err := c.GetServerIdentity(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			Expect(identity1.MachineIdentifier).To(Equal(identity2.MachineIdentifier))
			Expect(identity1.Version).To(Equal(identity2.Version))
		})
	})

	Describe("Ping", func() {
		It("pings server successfully", func(ctx SpecContext) {
			c := newPlexClient(server)

			err := c.Ping(ctx, server)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails for unreachable server", func(ctx SpecContext) {
			unreachableServer := plex.Server{
				Name:    "Unreachable Server",
				Address: "192.0.2.1",
				Port:    32400,
				Token:   testToken,
				Scheme:  "http",
			}

			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    testToken,
				Timeout:  5 * time.Second,
				BaseURL:  "http://192.0.2.1:32400",
			})

			err := c.Ping(ctx, unreachableServer)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetSessions", func() {
		It("returns sessions", func(ctx SpecContext) {
			c := newPlexClient(server)

			sessions, err := c.GetSessions(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(sessions).NotTo(BeNil())
		})
	})

	Describe("DiscoverServers", func() {
		It("discovers servers with token", func(ctx SpecContext) {
			token := os.Getenv("PLEX_TOKEN")
			Expect(token).NotTo(BeEmpty())

			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    token,
				Timeout:  30 * time.Second,
				BaseURL:  plexBaseURL,
			})

			servers, err := c.DiscoverServers(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).NotTo(BeNil())
		})
	})

	Describe("ValidateToken", func() {
		It("validates valid token", func(ctx SpecContext) {
			c := newPlexClient(server)

			valid, user, err := c.ValidateToken(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
			Expect(user).NotTo(BeNil())
			Expect(user.Title).NotTo(BeEmpty())
		})

		It("rejects invalid token", func(ctx SpecContext) {
			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    "invalid-token-12345",
				Timeout:  30 * time.Second,
				BaseURL:  plexBaseURL,
			})

			valid, _, err := c.ValidateToken(ctx)
			Expect(err).To(HaveOccurred())
			Expect(valid).To(BeFalse())
		})
	})

	Describe("GeneratePIN", func() {
		It("generates a PIN", func(ctx SpecContext) {
			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    "",
				Timeout:  30 * time.Second,
				BaseURL:  plexBaseURL,
			})

			pin, err := c.GeneratePIN(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(pin.ID).NotTo(BeZero())
			Expect(pin.Code).NotTo(BeEmpty())
		})

		It("PollPIN returns not yet claimed error", func(ctx SpecContext) {
			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    "",
				Timeout:  30 * time.Second,
				BaseURL:  plexBaseURL,
			})

			pin, err := c.GeneratePIN(ctx)
			Expect(err).NotTo(HaveOccurred())

			_, pollErr := c.PollPIN(ctx, pin.ID, pin.Code)
			if pollErr != nil && strings.Contains(pollErr.Error(), "invalid character '<'") {
				Skip("Plex API returned HTML instead of JSON for PIN polling endpoint")
			}

			Expect(pollErr).To(MatchError(plex.ErrPINNotYetClaimed))
		})
	})

	Describe("GetMedia", func() {
		It("returns media for first library", func(ctx SpecContext) {
			c := newPlexClient(server)

			libs, err := c.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libs).NotTo(BeEmpty())

			items, err := c.GetMedia(ctx, server, libs[0].ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(items).NotTo(BeNil())
		})

		It("returns media with data", func(ctx SpecContext) {
			c := newPlexClient(server)

			libs, err := c.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libs).NotTo(BeEmpty())

			items, err := c.GetMedia(ctx, server, libs[0].ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(items).NotTo(BeEmpty())

			for _, item := range items {
				Expect(item.ID).NotTo(BeEmpty())
				Expect(item.Title).NotTo(BeEmpty())
				Expect(item.Type).NotTo(BeEmpty())
				Expect(item.Duration).To(BeNumerically(">=", 0))
			}
		})

		It("returns empty for invalid library ID", func(ctx SpecContext) {
			c := newPlexClient(server)

			items, err := c.GetMedia(ctx, server, "999999999")
			if err != nil {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(items).To(BeEmpty())
			}
		})

		It("returns media for multiple libraries", func(ctx SpecContext) {
			c := newPlexClient(server)

			libs, err := c.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libs).NotTo(BeEmpty())

			for _, lib := range libs {
				items, ferr := c.GetMedia(ctx, server, lib.ID)
				Expect(
					ferr,
				).NotTo(HaveOccurred(), "should be able to fetch media for library %s", lib.ID)
				Expect(items).NotTo(BeNil(), "media items should not be nil for library %s", lib.ID)
			}
		})
	})

	Describe("GetMediaPath", func() {
		It("returns valid media path", func(ctx SpecContext) {
			c := newPlexClient(server)

			libs, err := c.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libs).NotTo(BeEmpty())

			items, err := c.GetMedia(ctx, server, libs[0].ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(items).NotTo(BeEmpty())

			path, err := c.GetMediaPath(ctx, server, items[0].ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(path).NotTo(BeEmpty())
			Expect(path).To(ContainSubstring("/"))
		})

		It("returns error for invalid media ID numeric", func(ctx SpecContext) {
			c := newPlexClient(server)

			_, err := c.GetMediaPath(ctx, server, "999999999")
			Expect(err).To(HaveOccurred())
		})

		It("returns error for invalid media ID format", func(ctx SpecContext) {
			c := newPlexClient(server)

			_, err := c.GetMediaPath(ctx, server, "invalid-id")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetAuthURL", func() {
		It("generates correct auth URL", func() {
			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    "",
				Timeout:  30 * time.Second,
				BaseURL:  plexBaseURL,
			})

			authURL := c.GetAuthURL("test-pin", "test-client", "http://localhost:8080/callback")

			Expect(authURL).To(ContainSubstring("app.plex.tv/auth"))
			Expect(authURL).To(ContainSubstring("clientID=test-client"))
			Expect(authURL).To(ContainSubstring("code=test-pin"))
			Expect(authURL).To(ContainSubstring("context%5Bdevice%5D%5Bproduct%5D=outtake-e2e"))
			Expect(
				authURL,
			).To(ContainSubstring("forwardUrl=http%3A%2F%2Flocalhost%3A8080%2Fcallback"))
		})

		It("handles empty params", func() {
			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    "",
				Timeout:  30 * time.Second,
				BaseURL:  plexBaseURL,
			})

			authURL := c.GetAuthURL("", "", "")

			Expect(authURL).To(ContainSubstring("app.plex.tv/auth"))
		})
	})

	Describe("MapPlexType", func() {
		DescribeTable("maps Plex types correctly",
			func(input, expected string) {
				result := plex.MapPlexType(input)
				Expect(result).To(Equal(expected))
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
	})

	Describe("Errors", func() {
		It("GetLibraries fails with invalid token", func(ctx SpecContext) {
			serverURL := os.Getenv("PLEX_SERVER_URL")
			Expect(serverURL).NotTo(BeEmpty())

			parsed, err := url.Parse(serverURL)
			Expect(err).NotTo(HaveOccurred())

			invalidServer := plex.Server{
				Name:    "Test Server",
				Address: parsed.Hostname(),
				Port:    443,
				Token:   "invalid-token-xyz",
				Scheme:  "https",
			}

			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    "invalid-token-xyz",
				Timeout:  30 * time.Second,
				BaseURL:  serverURL,
			})

			_, err = c.GetLibraries(ctx, invalidServer)
			Expect(err).To(HaveOccurred())
		})

		It("GetLibraries fails for unreachable server", func(ctx SpecContext) {
			unreachableServer := plex.Server{
				Name:    "Unreachable Server",
				Address: "192.0.2.1",
				Port:    32400,
				Token:   testToken,
				Scheme:  "http",
			}

			c := plex.NewClient(plex.ClientConfig{
				Product:  outtakeE2E,
				ClientID: outtakeE2ETest,
				Token:    testToken,
				Timeout:  5 * time.Second,
				BaseURL:  "http://192.0.2.1:32400",
			})

			_, err := c.GetLibraries(ctx, unreachableServer)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Full Workflow", func() {
		It("completes full Plex workflow", func(ctx SpecContext) {
			c := newPlexClient(server)

			identity, err := c.GetServerIdentity(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.MachineIdentifier).NotTo(BeEmpty())

			err = c.Ping(ctx, server)
			Expect(err).NotTo(HaveOccurred())

			libs, err := c.GetLibraries(ctx, server)
			Expect(err).NotTo(HaveOccurred())
			Expect(libs).NotTo(BeEmpty())

			mainLib := libs[0]
			items, err := c.GetMedia(ctx, server, mainLib.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(items).NotTo(BeEmpty())

			item := items[0]
			path, err := c.GetMediaPath(ctx, server, item.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(path).NotTo(BeEmpty())

			GinkgoWriter.Printf("Full workflow completed: server=%s, library=%s, item=%s, path=%s",
				identity.MachineIdentifier, mainLib.Title, item.Title, path)
		})
	})
})

// getPlexServerConfig loads Plex server settings from the e2e environment.
//
// Returns:
//   - server: Configured Plex server.
func getPlexServerConfig() plex.Server {
	GinkgoHelper()

	token := os.Getenv("PLEX_TOKEN")
	Expect(token).NotTo(BeEmpty(), "PLEX_TOKEN not set, skipping e2e tests")

	serverURL := os.Getenv("PLEX_SERVER_URL")
	Expect(serverURL).NotTo(BeEmpty(), "PLEX_SERVER_URL not set, skipping e2e tests")

	parsed, err := url.Parse(serverURL)
	Expect(err).NotTo(HaveOccurred())

	port := 443
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		Expect(err).NotTo(HaveOccurred())
	}

	return plex.Server{
		Name:    "E2E Test Server",
		Address: parsed.Hostname(),
		Port:    port,
		Token:   token,
		Scheme:  parsed.Scheme,
	}
}

// newPlexClient constructs a Plex API client for server.
//
// Parameters:
//   - server: Plex server connection details.
//
// Returns:
//   - client: Plex API client.
func newPlexClient(server plex.Server) *plex.Client {
	GinkgoHelper()

	return plex.NewClient(plex.ClientConfig{
		Product:  outtakeE2E,
		ClientID: outtakeE2ETest,
		Token:    server.Token,
		Timeout:  30 * time.Second,
		BaseURL:  plexBaseURL,
	})
}
