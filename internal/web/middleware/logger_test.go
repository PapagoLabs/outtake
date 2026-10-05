// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/logging"
)

// logSink is the writer the package-level logger records into. A mutex keeps the
// buffer safe while the tests that share it run in parallel.
type logSink struct {
	mu   sync.Mutex
	held bytes.Buffer
}

// logRecords collects the lines the package-level logger writes.
var logRecords = &logSink{}

// String returns everything recorded so far.
func (s *logSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.held.String()
}

// Write records one log line.
func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	written, err := s.held.Write(p)
	if err != nil {
		return written, fmt.Errorf("record log line: %w", err)
	}

	return written, nil
}

// TestMain installs a capturing logger so the middleware under test records
// somewhere a white-box test can read it.
//
// Parameters:
//   - m: The test entry point for this package.
//
// Returns:
//   - code: The process exit code.
func TestMain(m *testing.M) {
	logging.Logger = zerolog.New(logRecords)

	os.Exit(m.Run())
}

func TestRequestLoggerPassesThroughAndRecordsTheRequest(t *testing.T) {
	t.Parallel()

	app := loggedApp(t, http.MethodGet, "/logger/records", okHandler())

	got := issueRequest(t, app, http.MethodGet, "/logger/records")

	require.Equal(t, fiber.StatusOK, got.status)

	line := recordFor(t, "/logger/records")

	require.NotEmpty(t, line, "the request is recorded")
	assert.Contains(t, line, `"message":"request"`)
	assert.Contains(t, line, `"method":"GET"`)
	assert.Contains(t, line, `"status":200`)
	assert.Contains(t, line, `"duration":`)
}

func TestRequestLoggerDoesNotSwallowTheRouteError(t *testing.T) {
	t.Parallel()

	app := loggedApp(t, http.MethodGet, "/logger/raises", teapotHandler())

	got := issueRequest(t, app, http.MethodGet, "/logger/raises")

	assert.Equal(t, fiber.StatusTeapot, got.status,
		"the route error still reaches the error handler")
	assert.NotEmpty(t, recordFor(t, "/logger/raises"),
		"a failing request is recorded too")
}

func TestRequestLoggerRecordsANonGetRequest(t *testing.T) {
	t.Parallel()

	app := loggedApp(t, http.MethodPost, "/logger/posts", okHandler())

	got := issueRequest(t, app, http.MethodPost, "/logger/posts")

	require.Equal(t, fiber.StatusOK, got.status)
	assert.Contains(t, recordFor(t, "/logger/posts"), `"method":"POST"`)
}

// loggedApp registers one route that passes through the request logger.
//
// Parameters:
//   - t: The test that owns the application.
//   - method: HTTP method the route answers.
//   - target: Path the route is registered at.
//   - last: The terminal handler the logger wraps.
//
// Returns:
//   - app: An application serving exactly that route.
func loggedApp(t *testing.T, method, target string, last fiber.Handler) *fiber.App {
	t.Helper()

	app := fiber.New()

	switch method {
	case http.MethodPost:
		app.Post(target, RequestLogger(), last)
	case http.MethodDelete:
		app.Delete(target, RequestLogger(), last)
	default:
		app.Get(target, RequestLogger(), last)
	}

	return app
}

// recordFor returns the log line recorded for one request path.
//
// Parameters:
//   - t: The test that read the record.
//   - path: Request path whose recorded line is wanted.
//
// Returns:
//   - line: The recorded line, or empty when nothing was recorded for the path.
func recordFor(t *testing.T, path string) string {
	t.Helper()

	for line := range strings.SplitSeq(logRecords.String(), "\n") {
		if strings.Contains(line, `"path":"`+path+`"`) {
			return line
		}
	}

	return ""
}
