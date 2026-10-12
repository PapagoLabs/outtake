// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

// TestShutdownEndsOpenLiveStreams covers a page stream open as the server
// shuts down.
//
// The server waits for open connections, so the stream has to end on its own
// for the shutdown to finish before its timeout.
func TestShutdownEndsOpenLiveStreams(t *testing.T) {
	t.Parallel()

	deps := testRouterDeps(t, testRouterDatabase(t))

	deps.Cfg.Env = config.EnvE2E

	job := &clip.Job{
		ID: "clip-1", Type: clip.TypeClip, Name: "Intro",
		Status: clip.StatusProcessing,
	}
	deps.Queue.Restore(job)

	app := New(deps)

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	served := make(chan error, 1)

	go func() {
		served <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"http://"+listener.Addr().
			String()+
			routes.PathLive+"?"+routes.QueryLiveClips+"="+job.ID,
		http.NoBody,
	)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	t.Cleanup(func() { _ = response.Body.Close() })

	require.Equal(t, fiber.StatusOK, response.StatusCode)

	reader := bufio.NewReader(response.Body)

	for {
		line, readErr := reader.ReadString('\n')
		require.NoError(t, readErr)

		if strings.HasPrefix(line, "event: ping") {
			break
		}
	}

	started := time.Now()

	require.NoError(t, app.ShutdownWithTimeout(5*time.Second))
	require.Less(t, time.Since(started), 2*time.Second, "the stream did not hold the shutdown")
	require.NoError(t, <-served)
}
