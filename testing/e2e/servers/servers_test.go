// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package servers

import (
	"net/http"
	"net/url"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Servers", func() {
	Describe("Page", func() {
		It("renders the chooser and its custom URL form", func(ctx SpecContext) {
			body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/servers"))

			Expect(body).To(ContainSubstring("<title>Servers - Outtake</title>"))
			Expect(body).To(ContainSubstring("Custom Server URL"))
			Expect(body).To(ContainSubstring(`name="customUrl"`))
		})

		It("reports that nothing was discovered without a session", func(ctx SpecContext) {
			body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/servers"))

			Expect(body).To(ContainSubstring("No Plex servers found for this account"))
		})
	})

	Describe("Selection", func() {
		It("redirects to the dashboard when a server URL is posted", func(ctx SpecContext) {
			testApp.SkipWithoutPlex()

			resp := testApp.PostForm(ctx, "/servers", url.Values{
				"customUrl": {os.Getenv(helpers.ServerURLEnv)},
				"token":     {testApp.Token},
				"name":      {"E2E Server"},
			})
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
			Expect(resp.Header.Get("Location")).To(HavePrefix("/"))
		})

		It("redirects with an error when no server URL is posted", func(ctx SpecContext) {
			resp := testApp.PostForm(ctx, "/servers", url.Values{})
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("js-flash"))
		})

		It("redirects with an error when the posted URL is unusable", func(ctx SpecContext) {
			resp := testApp.PostForm(ctx, "/servers", url.Values{
				"customUrl": {"not a url"},
			})
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("js-flash"))
		})
	})
})
