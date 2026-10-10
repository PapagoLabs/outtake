// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package failure describes what went wrong for the person who hit it: a
// plain message picked from the domain packages' sentinels and typed errors,
// and, when the message is not enough, details to report an issue with. The
// details carry the version, the time, a short reference that ties them to a
// log line, what failed, and the error chain scrubbed of Plex tokens.
//
// The web layer uses it for request failures and the app for failed renders,
// so pages, clip cards, and previews name the same failure the same way.
package failure
