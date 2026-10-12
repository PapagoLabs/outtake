// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package live serves the event stream that keeps a page current.
//
// A page opens one stream and names what it follows in the query, such as its
// rendering clips, its preview, and its playback panel. Each feed reports what
// changed once a second, and the stream sends it as a server-sent event. One
// stream per page keeps to one of the few connections a browser holds to the
// server.
package live
