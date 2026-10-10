// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package profiles

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Profiles", func() {
	Describe("Page", func() {
		It("renders the settings page and its new profile form", func(ctx SpecContext) {
			body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/settings/profiles"))

			Expect(body).To(ContainSubstring("Clip Profiles"))
			Expect(body).To(ContainSubstring("New Profile"))
			Expect(body).To(ContainSubstring("Add Profile"))
		})

		It("offers every export width", func(ctx SpecContext) {
			body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/settings/profiles"))

			Expect(body).To(ContainSubstring("720p"))
			Expect(body).To(ContainSubstring("1080p"))
			Expect(body).To(ContainSubstring("1440p"))
			Expect(body).To(ContainSubstring("4K"))
		})

		It("offers every libx264 preset", func(ctx SpecContext) {
			body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/settings/profiles"))

			Expect(body).To(ContainSubstring(`value="veryfast"`))
			Expect(body).To(ContainSubstring(`value="veryslow"`))
		})
	})

	Describe("Create", func() {
		It("stores a profile and lists it", func(ctx SpecContext) {
			createProfile(ctx, "E2E Created Profile")

			body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/settings/profiles"))

			Expect(body).To(ContainSubstring("E2E Created Profile"))
			Expect(profileIDFor(body, "E2E Created Profile")).NotTo(BeEmpty())
		})

		It("redirects with an error when the name is empty", func(ctx SpecContext) {
			resp := testApp.PostForm(ctx, "/settings/profiles", profileForm(url.Values{"name": {""}}))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("Enter a name"))
		})

		It("redirects with an error when the CRF is out of range", func(ctx SpecContext) {
			resp := testApp.PostForm(ctx, "/settings/profiles", profileForm(url.Values{"crf": {"99"}}))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("CRF must be 0 to 51"))
		})

		It("redirects with an error when the preset is unknown", func(ctx SpecContext) {
			resp := testApp.PostForm(ctx, "/settings/profiles", profileForm(url.Values{"preset": {"turbo"}}))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("Choose an encoder preset from the list"))
		})

		It("redirects with an error when the width is not an export size", func(ctx SpecContext) {
			resp := testApp.PostForm(
				ctx,
				"/settings/profiles",
				profileForm(url.Values{"maxWidth": {"123"}}),
			)
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("Choose a maximum resolution from the list"))
		})
	})

	Describe("Default", func() {
		It("marks a stored profile as the default", func(ctx SpecContext) {
			profileID := createProfile(ctx, "E2E Default Profile")

			resp := testApp.PostForm(ctx, fmt.Sprintf("/settings/profiles/%s/default", profileID), nil)
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			Expect(resp.Header.Get("Location")).To(HavePrefix("/settings/profiles"))
			Expect(testApp.Landing(ctx, resp)).NotTo(ContainSubstring("js-flash"))
		})

		It("redirects with an error for an unknown profile", func(ctx SpecContext) {
			resp := testApp.PostForm(ctx,
				"/settings/profiles/00000000000000000000000000000000/default", nil)
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("That profile no longer exists"))
		})
	})

	Describe("Delete", func() {
		It("removes a stored profile once another one remains", func(ctx SpecContext) {
			createProfile(ctx, "E2E Keeper Profile")

			profileID := createProfile(ctx, "E2E Deleted Profile")

			resp := testApp.PostForm(ctx, fmt.Sprintf("/settings/profiles/%s/delete", profileID), nil)
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			Expect(testApp.Landing(ctx, resp)).NotTo(ContainSubstring("js-flash"))

			body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/settings/profiles"))
			Expect(body).NotTo(ContainSubstring("E2E Deleted Profile"))
		})

		It("redirects with an error for an unknown profile", func(ctx SpecContext) {
			createProfile(ctx, "E2E Unknown Target Keeper")

			resp := testApp.PostForm(ctx,
				"/settings/profiles/00000000000000000000000000000000/delete", nil)
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			Expect(testApp.Landing(ctx, resp)).To(ContainSubstring("That profile no longer exists"))
		})
	})
})

// profileForm builds a valid clip profile form, with overrides applied.
func profileForm(overrides url.Values) url.Values {
	values := url.Values{
		"name":      {"E2E Profile"},
		"crf":       {"23"},
		"preset":    {"veryfast"},
		"audioKbps": {"128"},
		"maxWidth":  {"1280"},
		"isDefault": {"1"},
	}

	maps.Copy(values, overrides)

	return values
}

// createProfile stores a profile under the given name and returns its id. The
// service refuses to remove the last profile, so specs leave the ones they make
// in place rather than trying to empty the table again.
func createProfile(ctx SpecContext, name string) string {
	GinkgoHelper()

	resp := testApp.PostForm(ctx, "/settings/profiles", profileForm(url.Values{"name": {name}}))
	helpers.CloseBody(resp)

	Expect(resp.StatusCode).To(Equal(http.StatusFound))
	Expect(testApp.Landing(ctx, resp)).NotTo(
		ContainSubstring("js-flash"),
		"the %q profile stored without an error",
		name,
	)

	body := helpers.ReadBody(testApp.Do(ctx, http.MethodGet, "/settings/profiles"))

	profileID := profileIDFor(body, name)
	Expect(profileID).NotTo(BeEmpty(), "the settings page carries an id for %q", name)

	return profileID
}

// profileIDFor reads the id of the stored profile the page renders under name.
func profileIDFor(body []byte, name string) string {
	sections := strings.Split(string(body), `action="/settings/profiles/`)

	for _, section := range sections[1:] {
		profileID, _, found := strings.Cut(section, `"`)
		if !found || !isProfileID(profileID) {
			continue
		}

		if strings.Contains(section, `value="`+name+`"`) {
			return profileID
		}
	}

	return ""
}

// isProfileID reports whether a path segment is a minted profile identifier.
func isProfileID(value string) bool {
	if len(value) != 32 {
		return false
	}

	for _, char := range value {
		switch {
		case char >= '0' && char <= '9', char >= 'a' && char <= 'f':
		default:
			return false
		}
	}

	return true
}
