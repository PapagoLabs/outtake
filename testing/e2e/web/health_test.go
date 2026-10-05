// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package web

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Health", func() {
	It("reports the service as ok", func(ctx SpecContext) {
		resp := testApp.Do(ctx, http.MethodGet, helpers.HealthPath)
		body := helpers.ReadBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		payload := helpers.DecodeObject(body)
		Expect(payload).To(HaveKeyWithValue("status", "ok"))
		Expect(payload).To(HaveKeyWithValue("service", "outtake"))
	})

	It("answers without a Plex token", func(ctx SpecContext) {
		resp := testApp.Do(ctx, http.MethodGet, helpers.HealthPath)
		helpers.CloseBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})
