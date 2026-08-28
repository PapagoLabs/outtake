// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Media", func() {
	Context("Search", func() {
		It("returns bad request when query is missing", func() {
			resp := doRequest(http.MethodGet, "/api/media/search")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			errResp := decodeErrorResponse(respBody)
			Expect(errResp.Error).To(Equal("missing_query"))
		})

		It("returns items when query is provided", func() {
			resp := doRequest(http.MethodGet, "/api/media/search?q=test")

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			result := decodeListResponse(respBody)
			Expect(result.Items).NotTo(BeNil())
		})
	})

	Context("Sessions", func() {
		It("returns sessions list", func() {
			resp := doRequest(http.MethodGet, "/api/sessions")

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var result []map[string]any

			Expect(json.Unmarshal(respBody, &result)).To(Succeed())
		})
	})

	Context("Auth", func() {
		It("starts login or reports an error", func() {
			resp := doRequest(http.MethodPost, "/api/auth/login")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Or(Equal(http.StatusOK), Equal(http.StatusBadGateway)))
		})

		It("redirects callback without a PIN session", func() {
			resp := doRequest(http.MethodGet, "/api/auth/callback")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
		})

		It("reports waiting status before authorization", func() {
			resp := doRequest(http.MethodGet, "/api/auth/status")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
