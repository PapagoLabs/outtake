// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Health", func() {
	It("returns ok status", func() {
		resp := doRequest(http.MethodGet, "/api/healthz")
		defer closeBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var result map[string]any

		err := json.NewDecoder(resp.Body).Decode(&result)
		Expect(err).NotTo(HaveOccurred())

		Expect(result["status"]).To(Equal("ok"))
		Expect(result["service"]).To(Equal("outtake"))
	})
})
