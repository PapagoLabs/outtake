// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build e2e

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/PapagoLabs/outtake/internal/api"
)

// The content types the suite posts.
const (
	// contentTypeJSON is the clip API content type.
	contentTypeJSON = "application/json"

	// contentTypeForm is the settings page form content type.
	contentTypeForm = "application/x-www-form-urlencoded"
)

// statusFailed is a clip that could not be rendered.
const statusFailed = "failed"

// The polling cadence a clip or preview status wait uses.
const (
	// pollInterval is how long a status wait sleeps between polls.
	pollInterval = 100 * time.Millisecond
)

// Do sends a request to the application under test and returns the response. The
// caller owns the response body.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - method: HTTP method.
//   - path: Path and query, relative to the application base URL.
//
// Returns:
//   - resp: Response the router produced.
func (a *App) Do(ctx context.Context, method, path string) *http.Response {
	ginkgo.GinkgoHelper()

	req := a.newRequest(ctx, method, path, "", nil)
	a.authorize(req)

	return a.send(req)
}

// DoBody sends a request carrying a body, using the content type given.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - method: HTTP method.
//   - path: Path and query, relative to the application base URL.
//   - contentType: Value of the Content-Type header.
//   - body: Request body, which the request takes ownership of.
//
// Returns:
//   - resp: Response the router produced.
func (a *App) DoBody(
	ctx context.Context,
	method, path, contentType string,
	body io.Reader,
) *http.Response {
	ginkgo.GinkgoHelper()

	req := a.newRequest(ctx, method, path, contentType, body)
	a.authorize(req)

	return a.send(req)
}

// PostJSON posts a JSON document to path.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - path: Path relative to the application base URL.
//   - body: Document to encode as JSON.
//
// Returns:
//   - resp: Response the router produced.
func (a *App) PostJSON(ctx context.Context, path string, body any) *http.Response {
	ginkgo.GinkgoHelper()

	encoded, err := json.Marshal(body)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "encode the %s request body", path)

	return a.DoBody(ctx, http.MethodPost, path, contentTypeJSON, bytes.NewReader(encoded))
}

// PostForm posts urlencoded form values to path.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - path: Path relative to the application base URL.
//   - values: Form values to encode.
//
// Returns:
//   - resp: Response the router produced.
func (a *App) PostForm(ctx context.Context, path string, values url.Values) *http.Response {
	ginkgo.GinkgoHelper()

	return a.DoBody(
		ctx,
		http.MethodPost,
		path,
		contentTypeForm,
		strings.NewReader(values.Encode()),
	)
}

// CreateClip submits a clip covering the whole generated test video and returns
// its job id.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - mediaID: Media id the clip is cut from.
//   - title: Title the clip is stored under.
//
// Returns:
//   - id: Identifier of the created clip job.
func (a *App) CreateClip(ctx context.Context, mediaID, title string) string {
	ginkgo.GinkgoHelper()

	resp := a.PostJSON(ctx, ClipsPath, api.ClipRequest{
		MediaID:    mediaID,
		MediaTitle: title,
		MediaType:  movieType,
		StartTime:  0,
		Duration:   VideoSeconds,
		Quality:    qualityLow,
		ClipType:   clipType,
	})

	body := ReadBody(resp)
	gomega.Expect(resp.StatusCode).To(gomega.Equal(http.StatusCreated), "create the %q clip", title)

	clip := DecodeClip(body)
	gomega.Expect(clip.ID).NotTo(gomega.BeEmpty(), "the created clip carries an id")

	return clip.ID
}

// WaitForClipStatus polls a clip until it reports the expected status, failing
// the spec when the job fails or the timeout elapses.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - jobID: Identifier of the clip job to poll.
//   - expected: Status the wait is looking for.
//   - timeout: How long the wait keeps polling.
func (a *App) WaitForClipStatus(
	ctx context.Context,
	jobID, expected string,
	timeout time.Duration,
) {
	ginkgo.GinkgoHelper()

	path := ClipStatusPath(jobID)
	deadline := time.Now().Add(timeout)

	for {
		clip := DecodeClip(ReadBody(a.Do(ctx, http.MethodGet, path)))

		if string(clip.Status) == expected {
			return
		}

		if string(clip.Status) == statusFailed {
			ginkgo.Fail("clip job failed: " + clip.Error)

			return
		}

		if time.Now().After(deadline) {
			ginkgo.Fail(
				fmt.Sprintf(
					"timed out after %s waiting for clip %s to reach %s, last status was %s",
					timeout, jobID, expected, clip.Status,
				),
			)

			return
		}

		sleep(ctx, pollInterval)
	}
}

// newRequest builds a request against the reserved base URL.
func (a *App) newRequest(
	ctx context.Context,
	method, path, contentType string,
	body io.Reader,
) *http.Request {
	ginkgo.GinkgoHelper()

	if body == nil {
		body = http.NoBody
	}

	req, err := http.NewRequestWithContext(ctx, method, a.BaseURL+path, body)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "build the %s %s request", method, path)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return req
}

// send passes a request through the application and returns its response.
func (a *App) send(req *http.Request) *http.Response {
	ginkgo.GinkgoHelper()

	resp, err := a.Application.Test(req)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "%s %s", req.Method, req.URL.Path)

	return resp
}

// authorize adds the CSRF handshake an unsafe request needs. The token is minted
// by a safe request and then submitted in the CSRF header while the matching
// cookie rides along, because the composition root's Test entry point keeps no
// cookie jar between requests.
func (a *App) authorize(req *http.Request) {
	ginkgo.GinkgoHelper()

	if !needsCSRFToken(req.Method) {
		return
	}

	token := a.mintCSRF(req.Context())
	if token == "" {
		return
	}

	req.Header.Set(csrf.HeaderName, token)

	//nolint:gosec // The suite talks to itself over loopback HTTP, where a Secure cookie would never be sent back.
	req.AddCookie(&http.Cookie{
		Name:     csrf.ConfigDefault.CookieName,
		Value:    token,
		SameSite: http.SameSiteLaxMode,
	})
}

// mintCSRF reads a freshly minted CSRF token off a safe request.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//
// Returns:
//   - token: The published token, empty when the response carried none.
func (a *App) mintCSRF(ctx context.Context) string {
	ginkgo.GinkgoHelper()

	resp := a.send(a.newRequest(ctx, http.MethodGet, HealthPath, "", nil))
	defer CloseBody(resp)

	for _, cookie := range resp.Cookies() {
		if cookie.Name == csrf.ConfigDefault.CookieName && cookie.Value != "" {
			return cookie.Value
		}
	}

	return ""
}

// needsCSRFToken reports whether the CSRF middleware protects a request method.
func needsCSRFToken(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

// sleep waits for the poll interval, or until the spec's context ends.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - interval: How long to wait.
func sleep(ctx context.Context, interval time.Duration) {
	ginkgo.GinkgoHelper()

	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// ReadBody reads a response body and closes it.
//
// Parameters:
//   - resp: Response whose body is read.
//
// Returns:
//   - body: The bytes the response carried.
func ReadBody(resp *http.Response) []byte {
	ginkgo.GinkgoHelper()

	defer CloseBody(resp)

	body, err := io.ReadAll(resp.Body)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "read the response body")

	return body
}

// CloseBody closes a response body.
//
// Parameters:
//   - resp: Response whose body is closed.
func CloseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	_ = resp.Body.Close()
}

// DecodeClip decodes a clip response body.
//
// Parameters:
//   - body: Bytes read off a clip endpoint.
//
// Returns:
//   - clip: The decoded clip.
func DecodeClip(body []byte) api.ClipResponse {
	ginkgo.GinkgoHelper()

	var clip api.ClipResponse
	gomega.Expect(json.Unmarshal(body, &clip)).To(gomega.Succeed(), "decode the clip response")

	return clip
}

// DecodeList decodes a media list response body.
//
// Parameters:
//   - body: Bytes read off a media endpoint.
//
// Returns:
//   - list: The decoded media list.
func DecodeList(body []byte) api.MediaListResponse {
	ginkgo.GinkgoHelper()

	var list api.MediaListResponse
	gomega.Expect(
		json.Unmarshal(body, &list),
	).To(gomega.Succeed(), "decode the media list response")

	return list
}

// DecodeError decodes an error response body.
//
// Parameters:
//   - body: Bytes read off a failing endpoint.
//
// Returns:
//   - failure: The decoded error.
func DecodeError(body []byte) api.ErrorResponse {
	ginkgo.GinkgoHelper()

	var failure api.ErrorResponse
	gomega.Expect(
		json.Unmarshal(body, &failure),
	).To(gomega.Succeed(), "decode the error response")

	return failure
}

// DecodeObject decodes a JSON object body generically.
//
// Parameters:
//   - body: Bytes read off a JSON endpoint.
//
// Returns:
//   - object: The decoded object.
func DecodeObject(body []byte) map[string]any {
	ginkgo.GinkgoHelper()

	var object map[string]any
	gomega.Expect(json.Unmarshal(body, &object)).To(gomega.Succeed(), "decode the JSON object")

	return object
}
