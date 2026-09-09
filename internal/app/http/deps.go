// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package http

import (
	fiber "github.com/gofiber/fiber/v3"
)

// HTMLPages is the HTML route surface mounted by the router.
type HTMLPages interface {
	Login(ctx fiber.Ctx) error
	Dashboard(ctx fiber.Ctx) error
	DashboardSessions(ctx fiber.Ctx) error
	Media(ctx fiber.Ctx) error
	Playback(ctx fiber.Ctx) error
	MediaItemClips(ctx fiber.Ctx) error
	MediaItem(ctx fiber.Ctx) error
	PreviewFile(ctx fiber.Ctx) error
	NavLibraries(ctx fiber.Ctx) error
	NewClip(ctx fiber.Ctx) error
	ClipFile(ctx fiber.Ctx) error
	ClipRow(ctx fiber.Ctx) error
	Clips(ctx fiber.Ctx) error
	Servers(ctx fiber.Ctx) error
	SelectServer(ctx fiber.Ctx) error
	Appearance(ctx fiber.Ctx) error
	ClipProfiles(ctx fiber.Ctx) error
	CreateClipProfile(ctx fiber.Ctx) error
	SetDefaultClipProfile(ctx fiber.Ctx) error
	DeleteClipProfile(ctx fiber.Ctx) error
	UpdateClipProfile(ctx fiber.Ctx) error
}

// ThumbPages serves cached Plex thumbnails.
type ThumbPages interface {
	Get(ctx fiber.Ctx) error
}

// ClipAPI is the JSON clip route surface.
type ClipAPI interface {
	Create(ctx fiber.Ctx) error
	Preview(ctx fiber.Ctx) error
	Update(ctx fiber.Ctx) error
	Cancel(ctx fiber.Ctx) error
	List(ctx fiber.Ctx) error
	GetStatus(ctx fiber.Ctx) error
	Download(ctx fiber.Ctx) error
	Delete(ctx fiber.Ctx) error
}

// MediaAPI is the JSON media route surface.
type MediaAPI interface {
	Search(ctx fiber.Ctx) error
	GetSessions(ctx fiber.Ctx) error
}

// AuthAPI is the JSON auth route surface.
type AuthAPI interface {
	Login(ctx fiber.Ctx) error
	Callback(ctx fiber.Ctx) error
	Status(ctx fiber.Ctx) error
	Logout(ctx fiber.Ctx) error
}
