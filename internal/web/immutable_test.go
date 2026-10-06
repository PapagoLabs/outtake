// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"
)

// keptValues are request strings a handler stored past the request.
type keptValues struct {
	// param is the route parameter.
	param string
	// form is the form value, read from the query string.
	form string
}

func TestNewConfiguresTheServer(t *testing.T) {
	t.Parallel()

	cfg := newBrowser(t).app.Config()

	assert.True(t, cfg.Immutable, "request strings are copied")
	assert.Equal(t, readTimeout, cfg.ReadTimeout)
	assert.Equal(t, idleTimeout, cfg.IdleTimeout)
	assert.Zero(t, cfg.WriteTimeout, "long downloads are not cut off")
}

func TestNewKeepsRequestStringsAfterALaterRequest(t *testing.T) {
	t.Parallel()

	app := newBrowser(t).app

	kept := make(chan keptValues, 2)

	app.Get("/probe/:id", func(ctx fiber.Ctx) error {
		kept <- keptValues{param: ctx.Params("id"), form: ctx.FormValue("v")}

		return ctx.SendStatus(fiber.StatusNoContent)
	})

	conn := serveOnLoopback(t, app)
	reader := bufio.NewReader(conn)

	probe(t, conn, reader, "/probe/first-id?v=first-value")

	first := <-kept

	probe(t, conn, reader, "/probe/later-id?v=later-value")
	<-kept

	assert.Equal(t, keptValues{param: "first-id", form: "first-value"}, first,
		"a later request on the same connection must not rewrite kept strings")
}

// serveOnLoopback serves an app on a loopback listener and dials one
// keep-alive connection to it.
//
// Parameters:
//   - t: The test that owns the server.
//   - app: The application to serve.
//
// Returns:
//   - conn: An open connection to the server, closed when the test finishes.
func serveOnLoopback(t *testing.T, app *fiber.App) net.Conn {
	t.Helper()

	listenConfig := &net.ListenConfig{}

	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	served := make(chan error, 1)

	go func() {
		served <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	t.Cleanup(func() {
		_ = app.ShutdownWithTimeout(time.Second)

		<-served
	})

	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(t.Context(), "tcp", listener.Addr().String())
	require.NoError(t, err)

	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// probe sends one GET over a kept-alive connection and drains its response.
//
// Parameters:
//   - t: The test that sends the request.
//   - conn: The connection to write to.
//   - reader: The buffered reader over conn.
//   - target: Request path and query.
func probe(t *testing.T, conn net.Conn, reader *bufio.Reader, target string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
	require.NoError(t, err)

	req.Host = "127.0.0.1"

	require.NoError(t, req.Write(conn))

	resp, err := http.ReadResponse(reader, req)
	require.NoError(t, err)

	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, fiber.StatusNoContent, resp.StatusCode)
}
