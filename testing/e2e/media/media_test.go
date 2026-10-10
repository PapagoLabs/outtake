// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package media

import (
	"encoding/json"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Media", func() {
	Describe("Search", func() {
		It("rejects a request that carries no query", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, "/api/media/search")
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(helpers.DecodeError(body).Error).To(Equal(api.MissingQuery))
		})

		It("returns a media list for a query", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, "/api/media/search?q=test")
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			result := helpers.DecodeList(body)
			Expect(result.Items).NotTo(BeNil())
			Expect(result.Total).To(Equal(len(result.Items)))
		})

		It("reaches Plex when credentials are configured", func(ctx SpecContext) {
			testApp.SkipWithoutPlex()

			resp := testApp.Do(ctx, http.MethodGet, "/api/media/search?q=test")
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			result := helpers.DecodeList(body)
			Expect(result.Items).NotTo(BeNil())
		})
	})

	Describe("Sessions", func() {
		It("returns a session list", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, "/api/sessions")
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var sessions []map[string]any
			Expect(json.Unmarshal(body, &sessions)).To(Succeed())
			Expect(sessions).NotTo(BeNil())
		})

		It("reaches Plex when credentials are configured", func(ctx SpecContext) {
			testApp.SkipWithoutPlex()

			resp := testApp.Do(ctx, http.MethodGet, "/api/sessions")
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
