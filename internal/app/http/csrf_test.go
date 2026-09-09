// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PapagoLabs/outtake/internal/config"
)

const publicOrigin = "https://clips.example"

func TestCSRFCookieSecureFollowsPublicURL(t *testing.T) {
	t.Parallel()

	httpCfg := &config.Config{ListenAddr: "127.0.0.1:8080"}
	assert.False(t, CSRFConfig(httpCfg).CookieSecure)

	httpsCfg := &config.Config{PublicBaseURL: publicOrigin}
	assert.True(t, CSRFConfig(httpsCfg).CookieSecure)
}

func TestCSRFTrustedOriginsFollowsPublicBaseURL(t *testing.T) {
	t.Parallel()

	local := CSRFConfig(&config.Config{ListenAddr: "127.0.0.1:8080"})
	assert.Empty(t, local.TrustedOrigins)

	behindTLS := CSRFConfig(&config.Config{PublicBaseURL: publicOrigin + "/"})
	assert.Equal(t, []string{publicOrigin}, behindTLS.TrustedOrigins)

	withPath := CSRFConfig(&config.Config{PublicBaseURL: publicOrigin + "/outtake"})
	assert.Equal(t, []string{publicOrigin}, withPath.TrustedOrigins)
}
