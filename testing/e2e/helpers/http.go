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
func (harness *App) Do(ctx context.Context, method, path string) *http.Response {
	ginkgo.GinkgoHelper()

	req := harness.newRequest(ctx, method, path, "", nil)
	harness.authorize(req)

	return harness.send(req)
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
func (harness *App) DoBody(
	ctx context.Context,
	method, path, contentType string,
	body io.Reader,
) *http.Response {
	ginkgo.GinkgoHelper()

	req := harness.newRequest(ctx, method, path, contentType, body)
	harness.authorize(req)

	return harness.send(req)
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
func (harness *App) PostJSON(ctx context.Context, path string, body any) *http.Response {
	ginkgo.GinkgoHelper()

	encoded, err := json.Marshal(body)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "encode the %s request body", path)

	return harness.DoBody(ctx, http.MethodPost, path, contentTypeJSON, bytes.NewReader(encoded))
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
func (harness *App) PostForm(ctx context.Context, path string, values url.Values) *http.Response {
	ginkgo.GinkgoHelper()

	return harness.DoBody(
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
func (harness *App) CreateClip(ctx context.Context, mediaID, title string) string {
	ginkgo.GinkgoHelper()

	resp := harness.PostJSON(ctx, ClipsPath, api.ClipRequest{
		MediaID:    mediaID,
		MediaTitle: title,
		MediaType:  movieType,
		StartTime:  0,
		Duration:   VideoSeconds,
		// An empty quality asks for the default profile, whose id the
		// migrations generate.
		Quality:  "",
		ClipType: clipType,
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
func (harness *App) WaitForClipStatus(
	ctx context.Context,
	jobID, expected string,
	timeout time.Duration,
) {
	ginkgo.GinkgoHelper()

	path := ClipStatusPath(jobID)
	deadline := time.Now().Add(timeout)

	for {
		clip := DecodeClip(ReadBody(harness.Do(ctx, http.MethodGet, path)))

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
func (harness *App) newRequest(
	ctx context.Context,
	method, path, contentType string,
	body io.Reader,
) *http.Request {
	ginkgo.GinkgoHelper()

	if body == nil {
		body = http.NoBody
	}

	req, err := http.NewRequestWithContext(ctx, method, harness.BaseURL+path, body)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "build the %s %s request", method, path)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return req
}

// send passes a request through the application and returns its response.
func (harness *App) send(req *http.Request) *http.Response {
	ginkgo.GinkgoHelper()

	resp, err := harness.Application.Test(req)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "%s %s", req.Method, req.URL.Path)

	return resp
}

// authorize adds the CSRF handshake an unsafe request needs. CSRF tokens live
// in the session, so a safe request through the session middleware opens a
// session and mints its token, and the unsafe request carries that session's
// cookies with the token in the CSRF header. The composition root's Test entry
// point keeps no cookie jar between requests, so each unsafe request opens its
// own session.
func (harness *App) authorize(req *http.Request) {
	ginkgo.GinkgoHelper()

	if !needsCSRFToken(req.Method) {
		return
	}

	token, cookies := harness.mintCSRF(req.Context())
	if token == "" {
		return
	}

	req.Header.Set(csrf.HeaderName, token)

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
}

// mintCSRF opens a session with a safe request and reads the CSRF token it
// minted.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//
// Returns:
//   - token: The session's token, empty when the response carried none.
//   - cookies: The session and CSRF cookies the response set.
func (harness *App) mintCSRF(ctx context.Context) (string, []*http.Cookie) {
	ginkgo.GinkgoHelper()

	resp := harness.send(harness.newRequest(ctx, http.MethodGet, HandshakePath, "", nil))
	defer CloseBody(resp)

	cookies := resp.Cookies()

	for _, cookie := range cookies {
		if cookie.Name == csrf.ConfigDefault.CookieName && cookie.Value != "" {
			return cookie.Value, cookies
		}
	}

	return "", nil
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

// Landing follows a redirect to the page it names, carrying the session the
// request and its response used, and returns that page's body. A failure a
// form post left in the session shows in the page's banner.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - resp: The redirect.
//
// Returns:
//   - body: The page the redirect lands on.
func (harness *App) Landing(ctx context.Context, resp *http.Response) string {
	ginkgo.GinkgoHelper()

	return harness.LandingAt(ctx, resp, resp.Header.Get("Location"))
}

// LandingAt opens a page with the session a response left, and returns that
// page's body. It follows a hand-off that sends the browser on from a script,
// such as the Plex popup's, which no Location header names.
//
// Parameters:
//   - ctx: Request context, canceled when the spec ends.
//   - resp: The response whose session is carried.
//   - path: The page to open.
//
// Returns:
//   - body: The page at path.
func (harness *App) LandingAt(ctx context.Context, resp *http.Response, path string) string {
	ginkgo.GinkgoHelper()

	req := harness.newRequest(ctx, http.MethodGet, path, "", nil)

	if resp.Request != nil {
		for _, cookie := range resp.Request.Cookies() {
			req.AddCookie(cookie)
		}
	}

	for _, cookie := range resp.Cookies() {
		req.AddCookie(cookie)
	}

	return string(ReadBody(harness.send(req)))
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
