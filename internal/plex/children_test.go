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

func TestIsContainerType(t *testing.T) {
	t.Parallel()

	assert.True(t, IsContainerType("show"))
	assert.True(t, IsContainerType("season"))
	assert.False(t, IsContainerType("episode"))
	assert.False(t, IsContainerType("movie"))
}

func TestGetChildren(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/library/metadata/10/children", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[
			{"ratingKey":"11","title":"Season 1","type":"season"},
			{"ratingKey":"12","title":"Season 2","type":"season"}
		]}}`))
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

	items, err := client.GetChildren(t.Context(), server, "10")
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "Season 1", items[0].Title)
	assert.Equal(t, "season", items[0].Type)
}
