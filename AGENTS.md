# Outtake

Plex clip manager. Go 1.27, Fiber v3, templ, HTMX, Cobra. App code lives under `internal/` (no `pkg/`). Composition root: `internal/app`. CLI: `internal/cmd`. Entrypoint: `main.go`.

## Commands

Taskfile, not Make. golangci config: `build/golangci-lint/golangci-lint.yaml`.

```bash
task templ          # required before lint/test/vet
task lint           # golangci-lint --fix
task lint-ci        # no --fix (what CI runs)
task vet
task test
task run            # go run . server start
task templ-watch    # templ proxy → :8080, cmd is server start
task compose-dev    # docker compose up --build
task mock           # mockery --config=build/mockery/mockery.yaml
```

Validate with `task lint-ci` then `task vet`, not `go build`. Single package: `go test ./internal/ffmpeg -run TestFoo` after `task templ`.

Server CLI is `outtake server start` (or `go run . server start`), not `serve`. Default listen `:8080`. Do not add a templ `:8080`→`:8090` proxy or a `docker-compose.dev.yml` overlay; one `docker-compose.yml` plus `.env`.

## Generated code

`**/*.templ.go` is gitignored. `task templ` is `templ generate` then `goimports -local github.com/PapagoLabs/outtake -w ./internal/web`. Ungrouped imports after generate are fixed by that goimports pass, not by skipping generate.

Do not add templ/goimports hooks to GoReleaser. CI and `task goreleaser*` already generate first. Do not edit Mockery output; regenerate with `task mock`.

CSS: `task tailwind` reads `internal/web/assets/css/input.css`.

## Test tiers

Three tiers. Keep a test in the lowest tier that can prove the thing.

- **Unit (white-box)** — `package foo`, `<file>_test.go`, beside the code. testify. Most tests live here.
- **Integration (black-box)** — `package foo_test`, `internal/<domain>/<package>_integration_test.go`. No build tag, so it runs in `go test ./...`. Exercises a package's exported surface against its real collaborators. It must not contact an external service: use `httptest`, `t.TempDir()`, the in-process SQLite in `internal/store/database`, or a mockery mock. `httptest` beats a mock at a process boundary you do not own; a mock beats one you do.
- **E2E** — `//go:build e2e` under `testing/e2e`, one subdirectory per app area, with the shared harness in `testing/e2e/helpers`. Not in `go test ./...`. Needs ffmpeg plus Plex credentials from `testing/e2e/.env` (see `.env.example`). A shell export beats the file. Specs skip cleanly when their prerequisite is absent, so a credential-less run is green, not red.

`internal/web/handlers` and `internal/web/middleware` accept narrow consumer-owned interfaces rather than concrete `*identity.Auth`, so tests drive them with `task mock` output instead of a live Plex.

## CI coverage gate

`.github/workflows/test.yaml` measures coverage one package at a time and aggregates; a single merged `-coverprofile` across `./...` cannot be summed. The gate reads only the **hand-written** bucket and excludes `**/mocks/**` and `*_templ.go`, which are scaffolding. Raise the floor as coverage improves; never lower it. It also runs the suite under `-race`.

## Nested module and e2e

`scripts/download-ffmpeg` is its own module (`outtake-scripts`). Test it with `go test -v` in that directory. Security CI scans it separately.

There is no `testing/integration` tree. `task test-e2e` runs the e2e suite.

## Release

- Dockerfiles: `build/docker/Dockerfile` (GoReleaser image context) and `Dockerfile.dev` (source build used by compose).
- GoReleaser: `build/goreleaser/stable.yaml` (git tag `vX.Y.Z`) and `nightly.yaml`. Docker `hooks.pre` runs `scripts/download-ffmpeg` into the image context. Ship ffmpeg/ffprobe binaries, not the downloader script.
- Images: `papagolabs/outtake` and `ghcr.io/papagolabs/outtake`.
- Migrations are `001_initial.sql`, `002_web_safe_color.sql`, `003_preserve_hdr.sql`, `004_users.sql`, and `005_sessions.sql`. SQLite reads `internal/store/database/migrations/`. Postgres reads `internal/store/database/migrations/postgres/`. The names match. Each connection records the files it has applied.
- IDs: Go stdlib `uuid`, not `github.com/google/uuid`.
- `References/` is local-only (gitignored).

## CI

Workflows call templ, goimports, and goreleaser directly, not Taskfile. Go lint is `.github/workflows/lint-go.yaml` (no `lint.yaml`). lint-go / test / vet / security are pull_request + path filters, not push. `lint-gh.yaml` only on `.github/workflows/**`. Stable release: exact `vX.Y.Z` tags, `cancel-in-progress: false`. Changelog: git-cliff via `update-changelog.yaml` on `main`.

## Domain

- Timecode is `internal/timecode` over Clock/FFmpegClock: constructors `FromSeconds` / `FromDuration` / `Parse`, and renderers `Duration` / `Short` / `String` / `FormatSeconds`. Parse through milliseconds. Library duration is HH:MM:SS; clip editing is HH:MM:SS.mmm. It is a top-level leaf: presentation and ffmpeg both depend on it, so nothing imports `internal/ffmpeg` just to format a duration.
- Render a `time.Duration` through `timecode.FromDuration(d).FormatSeconds()`, never `timecode.FormatSeconds(d.Seconds())`. The free `FormatSeconds(float64)` exists for wire seconds off a JSON body; going through it with a `time.Duration` unwraps and re-wraps the primitive for nothing.
- FFmpeg invocation is `internal/ffmpeg` with `probe/`, `crop/`, `progress/`, and `tonemap/` subpackages. Tests fake ffmpeg and ffprobe with `ffmpeg/ffmpegtest`: `ffmpegtest.Install` returns a symlink to the running test binary plus a `Stub` describing what it does, and `ffmpegtest.Dispatch()` goes first in the package's `TestMain` to act the stub out. Never write executable stub scripts, because running a file another parallel test's fork still holds open for writing fails with "text file busy". There is no published ffmpeg interface; callers take `*ffmpeg.ExecFFmpeg`. Name that parameter `runner` in a file that imports the package, so the import is not shadowed.
- ffmpeg children run with working directory `/`. Pass them absolute media paths.
- Black-bar trim uses `cropdetect=limit=24/255` after `format=yuv420p`. A bare `24` is 24/65535 on FFmpeg 9 10-bit HDR and misses letterboxing.
- Web-safe color is off by default and is the only switch that tone-maps HDR. Off keeps the source transfer (10-bit PQ/HLG, no tonemap), including when PreserveHDR is also off. On tone-maps on CPU with `zscale`+`tonemap=hable`. When both flags are set, web-safe wins. Sample luma with `format=yuv420p` before `signalstats`, so the peak math sees 8-bit limited codes, and use that measured luma. Leave libplacebo (Vulkan/GPU) out. A preview preset keeps the caller's PreserveHDR flag.
- Export max resolution is a clip-profile setting (720p, 1080p, 1440p, 4K). Defaults: Low 720p, Medium 1080p, High 4K. Preview stays 720p. GIF scale widths are even, because the chain ends in `yuv420p`.
- `clip.Clip` is the editable record. `clip.Job` embeds it and owns `InputPath`, `OutputPath`, `Status`, `Progress`, and `Error`.
- plex.tv calls decoded as XML (`/api/resources`, `/search`, `/status/sessions`) send `Accept: application/xml`. JSON calls stay on `doRequest`.
- `RemapMediaPath` treats the Plex root as a directory boundary, so `/data/media` does not claim `/data/media-other`. A mapping that would leave `LocalMediaRoot` is empty.
- `max-clip-dur` and `session-poll-sec` are second counts. An environment value arrives as a string and still means seconds.
- The clip queue keeps its own copy of every job and hands workers snapshots. Waiting jobs are ids in submission order, so `Submit` and `Requeue` never block. Status and progress reach the database through the queue's status callback, which reports one change at a time and always writes the latest state.
- Persisted clips reload before the server listens, unfinished ones oldest first, so every request sees every clip and a delete cannot be undone by a late reload.
- `num-workers` is clamped to at least 1 when configuration loads.
- Auth is single-owner. The first Plex account to sign in claims the `users` row with role `owner`. When `plex_tokens` holds a token, only the account behind the newest one may claim. `identity.Auth.SignIn` refuses every other account and never stores or binds a refused or invalid token. Sessions only gain a token through sign-in. The session carries the token and `plex_user_id`, and `middleware.AuthGuard` resolves that id against `users` on every request, so deleting the row (`outtake owner reset`) revokes every session. Logout ends one session. **Forget server** clears the binding. Keep `users` and its roles forward-compatible with a later multi-user model.
- `middleware.HostGuard` answers 421 to a Host that is not an IP literal, localhost, a dotless or private-suffix name, the public base URL's host, or listed in `allowed-hosts`. Tests that drive the router through httptest (Host `example.com`) set `AllowedHosts: "example.com"`.
- The e2e environment skips the auth guard, so `app.New` refuses it on a non-loopback listen address.
- Logging is configured once in `cmd/server` (and the e2e harness), not in `app.New`. Tests build many apps in parallel, and rewriting zerolog's globals while another app logs is a data race.
- Sessions persist in the `sessions` table through `database.SessionStore`, and CSRF tokens live in the session, so both survive a restart. `app` sweeps expired sessions. `/assets` (including a missing asset, which answers 404) and `/api/healthz` are mounted before the session middleware, so they never create a session.
- The Fiber app runs with `Immutable: true`, so strings read off a request (`Params`, `FormValue`, `Query`) are copies a handler may keep. It also sets a read and an idle timeout, and no write timeout, because downloads and ranged video stream for as long as they need.
- Vendored HTMX is v4. Partials are `<template hx type="partial" hx-target="...">`. A non-JSON error sets `HX-Reswap: none`, because every status other than 204 and 304 still swaps. The failure event is `htmx:response:error`.

## Layout

`internal/` is grouped by seam:

- `clip/` is the clip record and the render job. `clip/profile`, `clip/catalog`, `clip/preview`, and `clip/queue` sit under it.
- `timecode/` is the shared duration type.
- `ffmpeg/` runs ffmpeg and ffprobe, with `probe/`, `crop/`, `progress/`, and `tonemap/`. `ffmpeg/ffmpegtest` holds the fakes tests run in their place.
- `plex/` is the Plex client, with `decode/`, `session/`, `library/`, and `identity/`.
- `api/` aliases `clip.Request` and `clip.Response`. Error codes and the other response payloads stay in `api`.
- `web/` is templ and htmx: `view/`, `pages/`, `components/`, `routes/`, `respond/`, `theme/`, `exportform/`, and `handlers/` (`library`, `clip`, `preview`, `server`, `auth`, `profile`, `home`, `health`). Handler packages match those directory names. Alias domain imports `clipdom`, `clippreview`, and `clipprofile`. `router.go` imports the handlers as `clips`, `previews`, and `profiles`. E2E packages under `testing/e2e/{clips,profiles,previews}` stay plural.
- `store/database` and `store/blob` are the database and the filesystem/S3 adapters.
- `settings/config` and `settings/flags` are process configuration and CLI flags.
- `app/` wires the concrete runner, client, database, and blob store. `cmd/` only binds flags and starts `app`. `logging/` and `metadata/` stay at the root.

No package under `clip/`, `ffmpeg/`, `plex/`, or `store/` may import `internal/web`. `web` and `cmd` are the edges: handlers decode, call one domain function, then encode. `app` is the only place that knows which backend is wired, and it passes that wiring to `web.New`.
