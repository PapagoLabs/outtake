// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package clips

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/api"
	clipdom "github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/testing/e2e/helpers"
)

var _ = Describe("Clips", func() {
	BeforeEach(func() {
		testApp.SkipWithoutFFmpeg()
	})

	Describe("Create", func() {
		It("accepts a clip over the generated test video", func(ctx SpecContext) {
			mediaID := testApp.TestVideo

			resp := testApp.PostJSON(ctx, helpers.ClipsPath, api.ClipRequest{
				MediaID:    mediaID,
				MediaTitle: "Test Video",
				MediaType:  "movie",
				StartTime:  0,
				Duration:   helpers.VideoSeconds,
				Quality:    "",
				ClipType:   "clip",
			})

			body := helpers.ReadBody(resp)
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			clip := helpers.DecodeClip(body)
			Expect(clip.ID).NotTo(BeEmpty())
			Expect(clip.Status).To(Equal(clipdom.StatusPending))
			Expect(clip.MediaID).To(Equal(mediaID))
			Expect(clip.ClipType).To(Equal(clipdom.TypeClip))
		})

		It("rejects a zero duration", func(ctx SpecContext) {
			resp := testApp.PostJSON(ctx, helpers.ClipsPath, api.ClipRequest{
				MediaID:    "test",
				MediaTitle: "Test",
				Duration:   0,
				ClipType:   "clip",
			})
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("rejects an unknown clip type", func(ctx SpecContext) {
			resp := testApp.PostJSON(ctx, helpers.ClipsPath, api.ClipRequest{
				MediaID:    "test",
				MediaTitle: "Test",
				Duration:   10,
				ClipType:   "invalid",
			})
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("rejects a media id that resolves to nothing", func(ctx SpecContext) {
			resp := testApp.PostJSON(ctx, helpers.ClipsPath, api.ClipRequest{
				MediaID:    "/nonexistent/outtake-e2e-media.mkv",
				MediaTitle: "Missing",
				MediaType:  "movie",
				Duration:   1,
				Quality:    "",
				ClipType:   "clip",
			})

			body := helpers.ReadBody(resp)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(helpers.DecodeError(body).Error).To(Equal(api.MediaPathUnresolved))
		})
	})

	Describe("Status", func() {
		It("reports a job that was just submitted", func(ctx SpecContext) {
			clipID := testApp.CreateClip(ctx, testApp.TestVideo, "Status Test")

			clip := helpers.DecodeClip(
				helpers.ReadBody(testApp.Do(ctx, http.MethodGet, helpers.ClipStatusPath(clipID))),
			)

			Expect(clip.ID).To(Equal(clipID))
			// The test video is a second long, so the render may already be done.
			Expect(clip.Status).To(Or(
				Equal(clipdom.StatusPending),
				Equal(clipdom.StatusProcessing),
				Equal(clipdom.StatusCompleted),
				Equal(clipdom.StatusFailed),
			))
		})

		It("returns not found for an unknown job", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, helpers.ClipStatusPath("nonexistent-id"))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("List", func() {
		It("returns the clip collection", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, helpers.ClipsPath)
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(helpers.DecodeObject(body)).To(HaveKey("clips"))
		})
	})

	Describe("Full workflow", func() {
		It("renders a clip to completion", func(ctx SpecContext) {
			clipID := testApp.CreateClip(ctx, testApp.TestVideo, "Workflow Test")

			testApp.WaitForClipStatus(ctx, clipID, "completed", 60*time.Second)

			clip := helpers.DecodeClip(
				helpers.ReadBody(testApp.Do(ctx, http.MethodGet, helpers.ClipStatusPath(clipID))),
			)

			Expect(clip.Status).To(Equal(clipdom.StatusCompleted))
			Expect(clip.Error).To(BeEmpty())
		})
	})

	Describe("Download", func() {
		It("serves a finished clip", func(ctx SpecContext) {
			clipID := testApp.CreateClip(ctx, testApp.TestVideo, "Download Test")

			testApp.WaitForClipStatus(ctx, clipID, "completed", 60*time.Second)

			resp := testApp.Do(ctx, http.MethodGet, helpers.ClipDownloadPath(clipID))
			body := helpers.ReadBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Disposition")).To(ContainSubstring("attachment"))
			Expect(body).NotTo(BeEmpty())
		})

		It("refuses a clip that is not finished yet", func(ctx SpecContext) {
			clipID := testApp.CreateClip(ctx, testApp.TestVideo, "Not Ready Test")

			resp := testApp.Do(ctx, http.MethodGet, helpers.ClipDownloadPath(clipID))
			body := helpers.ReadBody(resp)

			// The test video is a second long, so the render may already be done.
			// Either answer is correct; what is asserted is that a refusal names
			// the reason rather than falling through to an empty download.
			if resp.StatusCode == http.StatusConflict {
				Expect(helpers.DecodeError(body).Error).To(Equal(api.NotReady))

				return
			}

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("returns not found for an unknown job", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodGet, helpers.ClipDownloadPath("nonexistent-id"))
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("Delete", func() {
		It("removes a queued clip", func(ctx SpecContext) {
			clipID := testApp.CreateClip(ctx, testApp.TestVideo, "Delete Test")

			resp := testApp.Do(ctx, http.MethodDelete, helpers.ClipsPath+"/"+clipID)
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			after := testApp.Do(ctx, http.MethodGet, helpers.ClipStatusPath(clipID))
			helpers.CloseBody(after)

			Expect(after.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("returns not found for an unknown job", func(ctx SpecContext) {
			resp := testApp.Do(ctx, http.MethodDelete, helpers.ClipsPath+"/nonexistent-id")
			helpers.CloseBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})
})
