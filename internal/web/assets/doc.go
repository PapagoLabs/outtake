// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package assets embeds the static files the web app serves under /assets, and
// names each one by a URL that carries a hash of its contents, so a browser can
// cache it for as long as the file is unchanged.
package assets
