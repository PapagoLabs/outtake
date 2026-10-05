// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package health

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingReader is a body that always fails part way through.
type failingReader struct{}

// loopbackAddr returns the listen address of a test server.
//
// Parameters:
//   - server: The running test server.
//
// Returns:
//   - addr: The address to hand to Check.
func loopbackAddr(server *httptest.Server) string {
	return strings.TrimPrefix(server.URL, "http://")
}

// healthServer serves one fixed response for every health check.
//
// Parameters:
//   - t: The test that needs the server.
//   - handler: The response the server gives.
//
// Returns:
//   - server: The running test server.
func healthServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return server
}

func TestNewCheckerUsesTheDefaultTimeout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, defaultHealthTimeout, NewChecker().timeout)
}

func TestCheckPassesAgainstAHealthyServer(t *testing.T) {
	t.Parallel()

	var requested string

	server := healthServer(t, func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	})

	require.NoError(t, NewChecker().Check(loopbackAddr(server)))
	assert.Equal(t, "/api/healthz", requested, "the checker probes the healthz route")
}

func TestCheckReportsANonOKStatus(t *testing.T) {
	t.Parallel()

	server := healthServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)

		_, _ = w.Write([]byte("shutting down"))
	})

	err := NewChecker().Check(loopbackAddr(server))
	require.ErrorIs(t, err, errHealthStatus)
	assert.ErrorContains(t, err, "status 503")
}

func TestCheckReportsAnUnreadableBody(t *testing.T) {
	t.Parallel()

	server := healthServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "64")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte("ok"))
	})

	err := NewChecker().Check(loopbackAddr(server))
	require.Error(t, err)
	assert.ErrorContains(t, err, "read response",
		"a body the server cut short is a read failure, not a status failure")
}

func TestCheckReportsAnUnreachableServer(t *testing.T) {
	t.Parallel()

	server := healthServer(t, func(http.ResponseWriter, *http.Request) {})
	addr := loopbackAddr(server)
	server.Close()

	err := NewChecker().Check(addr)
	require.Error(t, err)
	assert.ErrorContains(t, err, "health check failed",
		"a server that never answers is reported as a transport failure")
}

func TestCheckReportsAMalformedAddress(t *testing.T) {
	t.Parallel()

	err := NewChecker().Check("\x7f")
	require.Error(t, err)
	assert.ErrorContains(t, err, "parse URL")
}

func TestCheckGivesUpOnASlowServer(t *testing.T) {
	t.Parallel()

	server := healthServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)

		_, _ = w.Write([]byte("ok"))
	})

	err := (&Checker{timeout: 50 * time.Millisecond}).Check(loopbackAddr(server))
	require.Error(t, err)
	assert.ErrorContains(t, err, "health check")
}

func TestCheckHealthResponseAcceptsAnOKStatus(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
	}

	require.NoError(t, checkHealthResponse(resp))
}

func TestCheckHealthResponseReportsANonOKStatus(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader("boom")),
	}

	err := checkHealthResponse(resp)
	require.ErrorIs(t, err, errHealthStatus)
	assert.ErrorContains(t, err, "status 500")
}

func TestCheckHealthResponseReportsABodyItCannotRead(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(failingReader{}),
	}

	err := checkHealthResponse(resp)
	require.Error(t, err)
	assert.ErrorContains(t, err, "read response")
}

// Read returns a partial read followed by an error.
//
// Parameters:
//   - p: Buffer the reader fills.
//
// Returns:
//   - n: One byte written before the failure.
//   - err: io.ErrUnexpectedEOF.
func (failingReader) Read(p []byte) (int, error) {
	p[0] = 'o'

	return 1, io.ErrUnexpectedEOF
}
