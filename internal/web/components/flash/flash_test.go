// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flash

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/web/view"
)

func TestBannerOmitsEmptyMessage(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Banner(view.NewNotice("")).Render(t.Context(), &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

// TestBannerShowsDetailsOnlyWhenThereAreSome covers the Details section: a
// failure with details offers them collapsed with a copy button, and a notice
// shows its message alone.
func TestBannerShowsDetailsOnlyWhenThereAreSome(t *testing.T) {
	t.Parallel()

	var withDetails strings.Builder

	require.NoError(t, Banner(view.Failure{
		Message: "Couldn't save the clip",
		Details: "Outtake dev · ref 1a2b3c",
	}).Render(t.Context(), &withDetails))

	body := withDetails.String()
	assert.Contains(t, body, "Couldn&#39;t save the clip")
	assert.Contains(t, body, `<details class="text-xs text-foreground" data-error-details>`)
	assert.Contains(t, body, "<summary")
	assert.Contains(t, body, ">Details</summary>")
	assert.Contains(t, body, "Outtake dev · ref 1a2b3c")
	assert.Contains(t, body, `class="js-copy-details`)
	assert.Contains(t, body, ">Copy</button>")

	var notice strings.Builder

	require.NoError(t, Banner(view.NewNotice("Enter a name")).Render(t.Context(), &notice))
	assert.Contains(t, notice.String(), "Enter a name")
	assert.NotContains(t, notice.String(), "data-error-details")
}

func TestPartialTargetsFlash(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Partial(view.NewNotice("This clip isn't rendering")).Render(t.Context(), &buf)
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, `<template hx type="partial" hx-target="#flash">`)
	assert.Contains(t, body, "This clip isn&#39;t rendering")
	assert.Contains(t, body, "js-flash")
}

func TestSlotHasFlashID(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := Slot(view.NewNotice("")).Render(t.Context(), &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `id="flash"`)
}
