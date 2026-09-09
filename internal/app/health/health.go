// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package health provides health check logic for the outtake server.
package health

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
)

// Checker performs health checks against the outtake server.
type Checker struct {
	timeout time.Duration
}

// defaultHealthTimeout is the timeout for the health check HTTP request.
const defaultHealthTimeout = 5 * time.Second

// errHealthStatus is returned when the health check receives a non-200 status.
var errHealthStatus = errors.New("health check failed: unexpected status")

// NewChecker creates a new health Checker.
//
// Returns:
//   - checker: A new health Checker.
func NewChecker() *Checker {
	return &Checker{timeout: defaultHealthTimeout}
}

// Check performs a health check against the given address.
//
// Parameters:
//   - addr: Host:port of the listening HTTP server.
//
// Returns:
//   - err: Wrapped failure from "health check".
func (chkr *Checker) Check(addr string) error {
	resp, err := chkr.doHealthCheck(addr)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			log.Warn().Err(cerr).Msg("failed to close health check response body")
		}
	}()

	err = checkHealthResponse(resp)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	return nil
}

// checkHealthResponse validates the health check response.
//
// Parameters:
//   - resp: HTTP response to validate or close.
//
// Returns:
//   - err: Wrapped failure such as "health check: read response" or "...:
//     status ...".
func checkHealthResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("health check failed: could not read response")

		return fmt.Errorf("health check: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("health check failed: unexpected status")

		return fmt.Errorf("%w: status %d", errHealthStatus, resp.StatusCode)
	}

	log.Info().
		Int("status_code", resp.StatusCode).
		Str("response", string(body)).
		Msg("health check passed")

	return nil
}

// doHealthCheck sends a GET request to the health endpoint.
//
// Parameters:
//   - addr: Host:port of the listening HTTP server.
//
// Returns:
//   - resp: HTTP response. Caller must close the body.
//   - err: Wrapped failure such as "health check: parse URL", "health check:
//     create request", or "health check failed".
func (chkr *Checker) doHealthCheck(addr string) (*http.Response, error) {
	reqURL, err := url.Parse("http://" + addr + "/api/healthz")
	if err != nil {
		return nil, fmt.Errorf("health check: parse URL: %w", err)
	}

	client := &http.Client{
		Transport:     http.DefaultTransport,
		CheckRedirect: nil,
		Jar:           nil,
		Timeout:       chkr.timeout,
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		reqURL.String(),
		http.NoBody,
	)
	if err != nil {
		return nil, fmt.Errorf("health check: create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", reqURL.String()).
			Msg("health check failed: could not reach server")

		return nil, fmt.Errorf("health check failed: %w", err)
	}

	return resp, nil
}
