// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package web

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Pages", func() {
	DescribeTable("renders a page with its marker",
		func(path, marker string) {
			resp := testApp.Do(context.Background(), http.MethodGet, path)
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "%s renders", path)
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/html"))
			Expect(string(body)).To(ContainSubstring(marker), "%s carries its marker", path)
		},
		Entry("dashboard", "/", "<title>Dashboard - Outtake</title>"),
		Entry("clips", "/clips", "<title>Clips - Outtake</title>"),
		Entry("media", "/media", "<title>Media Library - Outtake</title>"),
		Entry("servers", "/servers", "<title>Plex Servers - Outtake</title>"),
		Entry("login", "/login", "Sign in with your Plex account"),
	)

	It("renders the dashboard shell", func(ctx SpecContext) {
		body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/"))

		Expect(string(body)).To(ContainSubstring("Outtake"))
		Expect(string(body)).To(ContainSubstring(`src="/assets/js/theme.js"`))
	})

	It("serves the security headers the helmet middleware sets", func(ctx SpecContext) {
		resp := testApp.Do(ctx, http.MethodGet, "/")
		helpers.CloseBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("X-Frame-Options")).To(Equal("DENY"))
		Expect(resp.Header.Get("X-Content-Type-Options")).To(Equal("nosniff"))
	})

	It("serves the embedded stylesheet", func(ctx SpecContext) {
		resp := testApp.Do(ctx, http.MethodGet, "/assets/css/output.css")
		helpers.CloseBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("returns not found for a path nothing serves", func(ctx SpecContext) {
		resp := testApp.Do(ctx, http.MethodGet, "/e2e/definitely-not-a-route")
		helpers.CloseBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})
})
