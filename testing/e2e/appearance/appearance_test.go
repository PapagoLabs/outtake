// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

// Package appearance holds the end-to-end specs for the theme palette page. The
// palette is chosen client side, so the page renders the full catalog and the
// specs check that every palette it advertises is present.
package appearance

import (
	"net/http"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/web/theme"
	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

// palettePattern matches each palette button the page renders.
var palettePattern = regexp.MustCompile(`data-palette="([^"]+)"`)

var _ = Describe("Appearance", func() {
	It("renders the appearance page", func(ctx SpecContext) {
		resp := testApp.Do(ctx, http.MethodGet, "/settings/appearance")
		body := helpers.ReadBody(resp)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/html"))
		Expect(body).To(ContainSubstring("Appearance"))
	})

	It("renders one button per catalog palette", func(ctx SpecContext) {
		body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/settings/appearance"))

		rendered := palettePattern.FindAllStringSubmatch(string(body), -1)
		Expect(rendered).To(HaveLen(len(theme.Palettes())))

		for _, palette := range theme.Palettes() {
			Expect(body).To(ContainSubstring(`data-palette="` + palette.ID + `"`))
			Expect(body).To(ContainSubstring(palette.Name))
		}
	})

	It("advertises the palettes to the theme script", func(ctx SpecContext) {
		body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/settings/appearance"))

		Expect(body).To(ContainSubstring(`data-palettes=`))
		Expect(body).To(ContainSubstring(`src="/assets/js/theme.js"`))
	})
})
