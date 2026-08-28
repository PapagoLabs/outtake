// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package plex

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: "test-client",
		Token:    "test-token",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})

	assert.Equal(t, productName, c.Product)
	assert.Equal(t, "test-client", c.ClientID)
	assert.Equal(t, "test-token", c.Token)
}

func TestNewClient_Defaults(t *testing.T) {
	t.Parallel()

	c := NewClient(ClientConfig{
		Product:  "",
		ClientID: "",
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})
	assert.Equal(t, productName, c.Product)
}

func TestSetToken(t *testing.T) {
	t.Parallel()

	c := NewClient(ClientConfig{
		Product:  "",
		ClientID: "",
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})
	c.SetToken("new-token")
	assert.Equal(t, "new-token", c.Token)
}

func TestDoRequest_SetsHeaders(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, productName, r.Header.Get("X-Plex-Product"))
		assert.Equal(t, "test-client", r.Header.Get("X-Plex-Client-Identifier"))
		assert.Equal(t, "test-token", r.Header.Get("X-Plex-Token"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		w.WriteHeader(http.StatusOK)
	}))

	defer ts.Close()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: "test-client",
		Token:    "test-token",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})

	c.baseURL, _ = url.Parse(ts.URL)

	ctx := t.Context()
	resp, err := c.doRequest(ctx, "/", "")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode())
}

func TestDecodeResponse_Error(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)

		_, _ = w.Write([]byte("bad request"))
	}))

	defer ts.Close()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: testServerClient,
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})

	c.baseURL, _ = url.Parse(ts.URL)

	resp, err := c.doRequest(t.Context(), "/", "")
	require.NoError(t, err)

	var data map[string]any

	err = c.decodeResponse(resp, &data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestGeneratePIN(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"id": 12345, "code": "abc123"}`))
	}))

	defer ts.Close()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: testServerClient,
		Token:    "",
		Timeout:  5 * time.Second,
		BaseURL:  "",
	})

	c.baseURL, _ = url.Parse(ts.URL)

	t.Logf("Base URL: %s", c.baseURL.String())

	pin, err := c.GeneratePIN(t.Context())
	if err != nil {
		t.Logf("Error: %v", err)
	}

	require.NoError(t, err)
	assert.Equal(t, 12345, pin.ID)
	assert.Equal(t, "abc123", pin.Code)
}

func TestPollPIN(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"authToken": "token-xyz"}`))
	}))

	defer ts.Close()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: testServerClient,
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})

	c.baseURL, _ = url.Parse(ts.URL)

	token, err := c.PollPIN(t.Context(), 12345, "testpin")
	require.NoError(t, err)
	assert.Equal(t, "token-xyz", token)
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"id": 1, "title": "Test User"}`))
	}))

	defer ts.Close()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: testServerClient,
		Token:    "valid",
		Timeout:  0,
		BaseURL:  "",
	})

	c.baseURL, _ = url.Parse(ts.URL)

	valid, user, err := c.ValidateToken(t.Context())
	require.NoError(t, err)
	assert.True(t, valid)
	assert.Equal(t, "Test User", user.Title)
}

func TestValidateToken_Invalid(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	defer ts.Close()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: testServerClient,
		Token:    "invalid",
		Timeout:  0,
		BaseURL:  "",
	})

	c.baseURL, _ = url.Parse(ts.URL)

	valid, _, err := c.ValidateToken(t.Context())
	require.ErrorIs(t, err, ErrUnauthorized)
	assert.False(t, valid)
}

func TestGetAuthURL(t *testing.T) {
	t.Parallel()

	c := NewClient(ClientConfig{
		Product:  productName,
		ClientID: "my-client",
		Token:    "",
		Timeout:  0,
		BaseURL:  "",
	})
	authURL := c.GetAuthURL("pin-code", "my-client", "http://localhost:8080/callback")
	assert.True(t, strings.HasPrefix(authURL, "https://app.plex.tv/auth#?"))
	assert.NotContains(t, authURL, "#%3F")
	assert.Contains(t, authURL, "clientID=my-client")
	assert.Contains(t, authURL, "code=pin-code")
	assert.Contains(t, authURL, "forwardUrl=http%3A%2F%2Flocalhost%3A8080%2Fcallback")
	assert.Contains(t, authURL, "context%5Bdevice%5D%5Bproduct%5D=")
}
