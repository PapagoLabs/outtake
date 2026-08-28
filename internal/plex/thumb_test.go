// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidThumbPath(t *testing.T) {
	t.Parallel()

	assert.True(t, ValidThumbPath("/library/metadata/1/thumb/2"))
	assert.False(t, ValidThumbPath("/thumb/100"))
	assert.False(t, ValidThumbPath("../etc/passwd"))
	assert.False(t, ValidThumbPath("/library/../etc/passwd"))
	assert.False(t, ValidThumbPath(""))
}

func TestGetThumb(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/metadata/1/thumb/2", r.URL.Path)
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte("jpeg-bytes"))
	}))
	defer ts.Close()

	host, port := extractAddrPort(t, ts)
	client := NewClient(ClientConfig{
		Product:  productName,
		ClientID: testServerClient,
		Token:    testSrvToken,
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})
	server := Server{
		Name:    testServerName,
		Address: host,
		Port:    port,
		Token:   testSrvToken,
		Scheme:  httpScheme,
		Local:   false,
	}

	body, contentType, err := client.GetThumb(t.Context(), server, "/library/metadata/1/thumb/2")
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", contentType)
	assert.Equal(t, []byte("jpeg-bytes"), body)

	_, _, err = client.GetThumb(t.Context(), server, "/etc/passwd")
	require.ErrorIs(t, err, ErrInvalidThumbPath)
}
