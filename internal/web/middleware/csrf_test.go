// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/plex/identity"
)

func TestBindCSRFTokenStoresTheFiberTokenOnTheRequestContext(t *testing.T) {
	t.Parallel()

	var upstream string

	app := chainApp(t, func(ctx fiber.Ctx) error {
		upstream = csrf.TokenFromContext(ctx)

		return ctx.SendString(identity.CSRFToken(ctx.Context()))
	}, csrf.New(), BindCSRFToken())

	got := issueRequest(t, app, http.MethodGet, "/dashboard/sessions")

	require.NotEmpty(t, got.body, "the token the templates read is set")
	assert.Equal(t, upstream, got.body)
	assert.Equal(t, fiber.StatusOK, got.status)
}

func TestBindCSRFTokenExposesTheTokenAsAnHXHeader(t *testing.T) {
	t.Parallel()

	app := chainApp(t, func(ctx fiber.Ctx) error {
		return ctx.SendString(identity.CSRFHeaderJSON(ctx.Context()))
	}, csrf.New(), BindCSRFToken())

	got := issueRequest(t, app, http.MethodGet, "/dashboard/sessions")

	assert.Contains(t, got.body, `{"X-Csrf-Token":"`)
	assert.Equal(t, fiber.StatusOK, got.status)
}

func TestBindCSRFTokenContinuesWhenNoCSRFTokenExists(t *testing.T) {
	t.Parallel()

	app := chainApp(t, func(ctx fiber.Ctx) error {
		return ctx.SendString(identity.CSRFToken(ctx.Context()))
	}, BindCSRFToken())

	got := issueRequest(t, app, http.MethodGet, "/clips")

	assert.Equal(t, fiber.StatusOK, got.status)
	assert.Empty(t, got.body)
}

func TestBindCSRFTokenBindsNothingOnARequestTheCSRFMiddlewareRefused(t *testing.T) {
	t.Parallel()

	app := chainApp(t, writeHandler("handler-reached"), csrf.New(), BindCSRFToken())

	got := issueRequest(t, app, http.MethodPost, "/dashboard/sessions")

	assert.Equal(t, fiber.StatusForbidden, got.status)
	assert.NotContains(t, got.body, "handler-reached",
		"the handler behind a refused request never runs")
}
