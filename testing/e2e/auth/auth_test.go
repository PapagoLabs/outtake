// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package auth

import (
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Auth", func() {
	Describe("Login", func() {
		It("starts PIN authorization or reports that Plex refused", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodPost, "/api/auth/login")
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Or(
				Equal(http.StatusOK), Equal(http.StatusBadGateway),
			))
		})

		It("hands back a plex.tv authorization URL", func(ctx SpecContext) {
			testApp.SkipWithoutPlex()

			resp := testApp.Do(ctx, http.MethodPost, "/api/auth/login")
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(helpers.DecodeObject(body)).To(HaveKey("authUrl"))
		})

		It("redirects a form post that carries no token", func(ctx SpecContext) {
			resp := testApp.PostForm(ctx, "/api/auth/login", url.Values{})
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("Paste a Plex token"))
		})

		It("refuses a token no Plex account holds", func(ctx SpecContext) {
			resp := testApp.PostForm(
				ctx,
				"/api/auth/login",
				url.Values{"token": {"invalid-token-12345"}},
			)
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		})
	})

	Describe("Callback", func() {
		It(
			"hands the popup back to the login page when no PIN session exists",
			func(ctx SpecContext) {
				resp := testApp.Do(ctx, http.MethodGet, "/api/auth/callback")
				body := helpers.ReadBody(resp)

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(string(body)).To(ContainSubstring(`data-auth-next="/login"`))
				Expect(testApp.LandingAt(ctx, resp, "/login")).
					To(ContainSubstring("This login expired. Start again from the login page."))
			},
		)
	})

	Describe("Status", func() {
		It("reports that it is waiting before authorization", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, "/api/auth/status")
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(string(body)).To(ContainSubstring("Waiting for Plex…"))
		})
	})

	Describe("Logout", func() {
		It("redirects to the login page", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodPost, "/api/auth/logout")
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
			Expect(resp.Header.Get("Location")).To(HavePrefix("/login"))
		})
	})
})
