#!/bin/sh
# Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
# SPDX-License-Identifier: AGPL-3.0-or-later
#
# Fails when the hand-written code's coverage is below the floor. Mockery mocks
# and templ output are scaffolding, so their lines are left out of the total.
# Raise the floor as coverage improves. Never lower it.
#
# Usage: build/ci/coverage-gate.sh coverage.out

set -eu

floor=90.5
profile=${1:?usage: coverage-gate.sh <coverage profile>}

handwritten=$(mktemp)
trap 'rm -f "$handwritten"' EXIT

grep -v -e '/mocks/' -e '_templ\.go:' "$profile" >"$handwritten"

go tool cover -func="$handwritten" | awk -v floor="$floor" '
	/^total:/ {
		seen = 1
		gsub(/%/, "", $3)
		if ($3 + 0 < floor + 0) {
			printf "hand-written coverage %.1f%% is below the %s%% floor\n", $3, floor
			exit 1
		}
		printf "hand-written coverage %.1f%% meets the %s%% floor\n", $3, floor
	}
	END {
		if (!seen) {
			print "go tool cover reported no total coverage"
			exit 1
		}
	}'
