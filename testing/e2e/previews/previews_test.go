// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package previews

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Previews", func() {
	Describe("Status", func() {
		It("returns not found for an id nothing rendered", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, statusPath("nothing-rendered"))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("returns not found for an unknown file", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, filePath("nothing-rendered"))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("returns not found for a malformed file id", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, filePath("../escape"))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("Request", func() {
		It("rejects a media id that resolves to nothing", func(ctx SpecContext) {
			resp := testApp.PostJSON(ctx, "/api/clips/preview", api.ClipRequest{
				MediaID:    "/nonexistent/outtake-e2e-media.mkv",
				MediaTitle: "Missing",
				MediaType:  "movie",
				StartTime:  0,
				Duration:   helpers.VideoSeconds,
				Quality:    "",
				ClipType:   "clip",
			})

			body := helpers.ReadBody(resp)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(helpers.DecodeError(body).Error).To(Equal(api.MediaPathUnresolved))
		})
	})

	Describe("Full workflow", func() {
		BeforeEach(func() {
			testApp.SkipWithoutFFmpeg()
		})

		It("renders a preview and then serves the file", func(ctx SpecContext) {
			previewID := submitPreview(ctx)

			waitForPreview(ctx, previewID)

			resp := testApp.Do(ctx, http.MethodGet, filePath(previewID))
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(body).NotTo(BeEmpty())
		})

		It("refuses to cancel a preview that already finished", func(ctx SpecContext) {
			previewID := submitPreview(ctx)

			waitForPreview(ctx, previewID)

			resp := testApp.Do(ctx, http.MethodDelete, statusPath(previewID))
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusConflict))
			Expect(helpers.DecodeError(body).Error).To(Equal(api.PreviewNotRunning))
		})

		It("returns not found when canceling an id nothing rendered", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodDelete, statusPath("nothing-rendered"))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})
})

// submitPreview requests a preview render and returns the id it redirects with.
func submitPreview(ctx SpecContext) string {
	GinkgoHelper()

	resp := testApp.PostJSON(ctx, "/api/clips/preview", api.ClipRequest{
		MediaID:    testApp.TestVideo,
		MediaTitle: "Preview Test",
		MediaType:  "movie",
		StartTime:  0,
		Duration:   helpers.VideoSeconds,
		Quality:    "",
		ClipType:   "clip",
	})
	helpers.CloseBody(resp)

	Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))

	location := resp.Header.Get("Location")
	Expect(location).To(ContainSubstring("preview="))

	return previewIDFrom(location)
}

// waitForPreview polls a preview until it publishes a file.
func waitForPreview(ctx SpecContext, previewID string) {
	GinkgoHelper()

	deadline := time.Now().Add(90 * time.Second)

	for {
		payload := helpers.DecodeObject(
			helpers.ReadBody(testApp.Do(ctx, http.MethodGet, statusPath(previewID))),
		)
		Expect(payload).To(HaveKeyWithValue("id", previewID))

		status, _ := payload["status"].(string)

		if status == "completed" {
			Expect(payload).To(HaveKeyWithValue("url", filePath(previewID)))

			return
		}

		if time.Now().After(deadline) {
			Fail(fmt.Sprintf(
				"timed out after %s waiting for preview %s, last status was %s",
				90*time.Second, previewID, status,
			))

			return
		}

		time.Sleep(250 * time.Millisecond)
	}
}

// previewIDFrom reads the preview id out of a preview redirect target.
func previewIDFrom(location string) string {
	GinkgoHelper()

	parsed, err := url.Parse(location)
	Expect(err).NotTo(HaveOccurred())

	previewID := parsed.Query().Get("preview")
	Expect(previewID).NotTo(BeEmpty(), "the redirect carries a preview id")

	return previewID
}

// statusPath builds the status endpoint for one preview.
func statusPath(previewID string) string {
	return "/api/clips/preview/" + previewID
}

// filePath builds the published preview file path for one preview.
func filePath(previewID string) string {
	return "/previews/" + previewID
}
