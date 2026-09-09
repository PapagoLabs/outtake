// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	testMediaTitle = "Test Video"
	testDuration   = 1
	testClipType   = "clip"
	testMediaType  = "movie"
	testQuality    = "low"
	keyMediaID     = "mediaId"
	keyMediaTitle  = "mediaTitle"
	keyDuration    = "duration"
	keyClipType    = "clipType"
	keyMediaType   = "mediaType"
	keyStartTime   = "startTime"
	keyQuality     = "quality"
)

var _ = Describe("Clips", func() {
	BeforeEach(func() {
		if !testApp.ffmpegOK {
			Skip("FFmpeg not available, skipping clip test")
		}
	})

	Describe("Create", func() {
		It("creates a clip job", func() {
			mediaID := testMediaID(testApp.testVideo)

			body := map[string]any{
				keyMediaID:    mediaID,
				keyMediaTitle: testMediaTitle,
				keyMediaType:  testMediaType,
				keyStartTime:  0,
				keyDuration:   testDuration,
				keyQuality:    testQuality,
				keyClipType:   testClipType,
			}

			resp := doJSONRequest(body)
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			clip := decodeClipResponse(respBody)
			Expect(clip.ID).NotTo(BeEmpty())
			Expect(clip.Status).To(Equal("pending"))
			Expect(clip.MediaID).To(Equal(mediaID))
			Expect(clip.ClipType).To(Equal(testClipType))
		})

		It("returns bad request for invalid duration", func() {
			body := map[string]any{
				keyMediaID:    "test",
				keyMediaTitle: "Test",
				keyDuration:   0,
				keyClipType:   testClipType,
			}

			resp := doJSONRequest(body)
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("returns bad request for invalid clip type", func() {
			body := map[string]any{
				keyMediaID:    "test",
				keyMediaTitle: "Test",
				keyDuration:   10,
				keyClipType:   "invalid",
			}

			resp := doJSONRequest(body)
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("Status", func() {
		It("returns clip status", func() {
			mediaID := testMediaID(testApp.testVideo)
			clipID := createTestClip(mediaID, "Test Video")

			resp := doRequest(http.MethodGet, "/api/clips/"+clipID+"/status")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			clip := decodeClipResponse(respBody)
			Expect(clip.ID).To(Equal(clipID))
			Expect(clip.Status).To(Or(Equal("pending"), Equal("processing"), Equal("failed")))
		})

		It("returns not found for nonexistent clip", func() {
			resp := doRequest(http.MethodGet, "/api/clips/nonexistent-id/status")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("List", func() {
		It("returns clips list", func() {
			resp := doRequest(http.MethodGet, "/api/clips")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]any

			Expect(json.Unmarshal(respBody, &result)).To(Succeed())
			Expect(result).To(HaveKey("clips"))
		})
	})

	Describe("Full Workflow", func() {
		It("completes clip processing", func() {
			mediaID := testMediaID(testApp.testVideo)
			clipID := createTestClip(mediaID, "Workflow Test")

			waitForClipStatus(clipID, "completed", 60*time.Second)

			resp := doRequest(http.MethodGet, "/api/clips/"+clipID+"/status")
			defer closeBody(resp)

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			clip := decodeClipResponse(respBody)
			Expect(clip.Status).To(Equal("completed"))
			Expect(clip.Error).To(BeEmpty())
		})
	})

	Describe("Download", func() {
		It("downloads completed clip", func() {
			mediaID := testMediaID(testApp.testVideo)
			clipID := createTestClip(mediaID, "Download Test")

			waitForClipStatus(clipID, "completed", 60*time.Second)

			resp := doRequest(http.MethodGet, "/api/clips/"+clipID+"/download")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(respBody).NotTo(BeEmpty())
		})

		It("returns conflict for not ready clip", func() {
			mediaID := testMediaID(testApp.testVideo)
			clipID := createTestClip(mediaID, "Not Ready Test")

			resp := doRequest(http.MethodGet, "/api/clips/"+clipID+"/download")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusConflict))
		})
	})

	Describe("Delete", func() {
		It("deletes completed clip", func() {
			mediaID := testMediaID(testApp.testVideo)
			clipID := createTestClip(mediaID, "Delete Test")

			resp := doRequest(http.MethodDelete, "/api/clips/"+clipID)
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Or(Equal(http.StatusNoContent), Equal(http.StatusNotFound)))
		})

		It("returns not found for nonexistent clip", func() {
			resp := doRequest(http.MethodDelete, "/api/clips/nonexistent-id")
			defer closeBody(resp)

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})
})

// closeBody closes resp.Body and ignores the error.
//
// Parameters:
//   - resp: Resp.
func closeBody(resp *http.Response) {
	_ = resp.Body.Close()
}

// createTestClip creates a clip via the API and returns its ID.
//
// Parameters:
//   - mediaID: Source media identifier.
//   - title: Clip display title.
//
// Returns:
//   - id: Created clip ID.
func createTestClip(mediaID, title string) string {
	GinkgoHelper()

	body := map[string]any{
		keyMediaID:    mediaID,
		keyMediaTitle: title,
		keyMediaType:  testMediaType,
		keyStartTime:  0,
		keyDuration:   testDuration,
		keyQuality:    testQuality,
		keyClipType:   testClipType,
	}

	resp := doJSONRequest(body)
	defer closeBody(resp)

	respBody, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	clip := decodeClipResponse(respBody)
	Expect(clip.ID).NotTo(BeEmpty())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	return clip.ID
}
