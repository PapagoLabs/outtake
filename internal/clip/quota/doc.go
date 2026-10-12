// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package quota bounds what a member may render: how many renders run at
// once, how many start in a rolling week, how much storage their clips take,
// and how long one clip may be. The owner and admins are not bound.
package quota
