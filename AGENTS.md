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

## E2E

There is no `testing/integration` tree. `task test-e2e` runs the e2e suite.

## Release

- Dockerfiles: `build/docker/Dockerfile` (GoReleaser image context) and `Dockerfile.dev` (source build used by compose). Both copy static `ffmpeg` and `ffprobe` from `mwader/static-ffmpeg`, pinned by its multi-arch digest in the `FFMPEG_IMAGE` build arg, which Renovate updates. The stage is pulled for the target platform, never `$BUILDPLATFORM`, so an arm64 image gets arm64 binaries. Development and production run the same FFmpeg.
- GoReleaser: `build/goreleaser/stable.yaml` (git tag `vX.Y.Z`) and `nightly.yaml`. The image context holds only the outtake binary.
- Images: `papagolabs/outtake` and `ghcr.io/papagolabs/outtake`.
- Migrations are `001_initial.sql`, `002_web_safe_color.sql`, `003_preserve_hdr.sql`, `004_users.sql`, `005_sessions.sql`, `006_keep_hdr.sql`, and `007_server_machine_id.sql`. SQLite reads `internal/store/database/migrations/`. Postgres reads `internal/store/database/migrations/postgres/`. The names match. Each connection records the files it has applied.
- IDs: Go stdlib `uuid`, not `github.com/google/uuid`.
- `References/` is local-only (gitignored).

## CI

Workflows call templ, goimports, and goreleaser directly, not Taskfile. Go lint is `.github/workflows/lint-go.yaml` (no `lint.yaml`). lint-go / test / vet / security are pull_request + path filters, not push. `lint-gh.yaml` only on `.github/workflows/**`. Stable release: exact `vX.Y.Z` tags, `cancel-in-progress: false`. Changelog: git-cliff via `update-changelog.yaml` on `main`.

## Domain

- Timecode is `internal/timecode` over Clock/FFmpegClock: constructors `FromSeconds` / `FromDuration` / `Parse`, and renderers `Duration` / `Short` / `String` / `FormatSeconds`. Parse through milliseconds. Library duration is HH:MM:SS; clip editing is HH:MM:SS.mmm. It is a top-level leaf: presentation and ffmpeg both depend on it, so nothing imports `internal/ffmpeg` just to format a duration.
- Render a `time.Duration` through `timecode.FromDuration(d).FormatSeconds()`, never `timecode.FormatSeconds(d.Seconds())`. The free `FormatSeconds(float64)` exists for wire seconds off a JSON body; going through it with a `time.Duration` unwraps and re-wraps the primitive for nothing.
- FFmpeg invocation is `internal/ffmpeg` with `probe/`, `crop/`, `progress/`, and `tonemap/` subpackages. Tests fake ffmpeg and ffprobe with `ffmpeg/ffmpegtest`: `ffmpegtest.Install` returns a symlink to the running test binary plus a `Stub` describing what it does, and `ffmpegtest.Dispatch()` goes first in the package's `TestMain` to act the stub out. Never write executable stub scripts, because running a file another parallel test's fork still holds open for writing fails with "text file busy". There is no published ffmpeg interface; callers take `*ffmpeg.ExecFFmpeg`. Name that parameter `runner` in a file that imports the package, so the import is not shadowed.
- ffmpeg children run with working directory `/`. Pass them absolute media paths.
- Every render writes to a `.staging-<uuid><ext>` file beside its output and is moved into place only when ffmpeg succeeds and the file is not empty, so a failed or canceled regenerate keeps the old file. GIF palettes sit beside the staging file. `app` sweeps staging files and palettes nothing has written for an hour from the output directories before any worker starts, so a render another process sharing the storage path is running keeps its file. A render canceled after ffmpeg finished is not published. An empty output path is refused.
- `progress.Writer` parses only what each write adds, keeps a 64 KiB tail of stderr for error logs, and reports a percent only when it changes. `app` records a render's progress at most once a second.
- SQLite opens through the `modernc.org/sqlite` DSN with WAL, `busy_timeout(5000)`, `synchronous(NORMAL)`, and immediate transactions, on a pool of four connections.
- `blob.Blob` has `Exists`, which never downloads, and `Ensure`, which downloads only a missing local copy. Cards and badges use `Exists`. Playback, download, previews, and the thumbnail cache use `Ensure`. Only a regular file counts. Thumbnails are cached per server (`library.CacheID(server, path)`), written atomically, and served `private`.
- Encodes pass `-abort_on empty_output_stream`, and clips and previews drop the source's metadata and chapters (`-map_metadata -1 -map_chapters -1`). On FFmpeg 9 `-map_metadata -1` drops per-stream tags too, which a real-ffmpeg test checks.
- An ffmpeg run's deadline is `ffmpeg-timeout-sec`, or with the default of 0, the larger of 30 minutes and 20 times the clip's length. A run stopped at its deadline returns `ffmpeg.ErrTimeout`, whose message names the setting.
- Black-bar trim uses `cropdetect=limit=24/255` after `format=yuv420p`. A bare `24` is 24/65535 on FFmpeg 9 10-bit HDR and misses letterboxing.
- Create, update, and preview check a selection through `library.MediaSource.CheckEdit`, which probes once and applies `clip.Edit.Validate`: the `max-clip-dur` cap, the source's end, and an audio track the source carries. A JSON mark below zero is refused when the request is parsed.
- A video clip's **Keep HDR** (`PreserveHDR`) is the only HDR switch. On keeps the source transfer (10-bit PQ/HLG, no tonemap), encoded as HEVC Main 10 (`libx265`) in an `hvc1` track at the profile CRF + 1 with the profile preset. Its color tags go in `-x265-params` as well as the output flags, because FFmpeg does not pass them on to libx265 for an untagged input, and PQ alone adds `hdr10-opt=1`. The source's HDR10 mastering display and light level data pass through unchanged, which a real-ffmpeg test checks through a crop and a scale, so never hard-code `master-display` or `max-cll`. Everything else is 8-bit H.264. Off tone-maps on CPU with `zscale`+`tonemap=mobius` to 8-bit BT.709, encoded and tagged with the BT.709 transfer (1-1-1), which upload sites and editors expect. GIFs and JPEGs are encoded with the sRGB transfer, because neither format carries a tag. A tone-mapped clip carries no mastering display or light level data, which a real-ffmpeg test checks on FFmpeg 8 and later. A new clip takes its profile's `keep_hdr` default (built-ins: High on, Low and Medium off) through `profile.Preset`, and a request's choice wins. GIFs and screenshots always tone-map an HDR source, between the crop and the scale, because neither format can carry HDR. The PQ peak comes from the source's MaxCLL, then its mastering display peak, read from ffprobe's stream side data, so clips of one title share their exposure. Without either, sample luma with `format=yuv420p` before `signalstats`, so the peak math sees 8-bit limited codes, and use that measured luma. Leave libplacebo (Vulkan/GPU) out. A preview of a clip that keeps HDR is tone-mapped unless the export form's script marked the browser as able to show it (`screenHdr`: the `video-dynamic-range` or `dynamic-range` media query matches and `canPlayType` accepts HEVC Main 10), and the page labels it. An HDR preview is HEVC like the clip. The preview id follows what the render does, while the redirect carries the clip's own choice back to the form. A clip card whose video the browser cannot decode says so, from the video's `error` event. Web-safe color is retired: `web_safe_color` is written as the inverse of `preserve_hdr` and never read, and the API reads a sent `webSafeColor` as the inverse of `preserveHdr`, which wins when both are sent.
- `library.MediaSource.CheckEdit` refuses a Dolby Vision source with no displayable base layer (profile 5, or base layer compatibility id 0), read from ffprobe's DOVI configuration record. ffmpeg decodes only the base layer, which for profile 5 is in Dolby's own color space.
- Export max resolution is a clip-profile setting (720p, 1080p, 1440p, 4K). Defaults: Low 720p, Medium 1080p, High 4K. Preview stays 720p. GIF scale widths are even, because the chain ends in `yuv420p`.
- `clip.Clip` is the editable record. `clip.Job` embeds it and owns `InputPath`, `OutputPath`, `Status`, `Progress`, and `Error`.
- plex.tv calls decoded as XML (`/api/resources`, `/search`, `/status/sessions`) send `Accept: application/xml`. JSON calls stay on `doRequest`.
- `RemapMediaPath` treats the Plex root as a directory boundary, so `/data/media` does not claim `/data/media-other`. A mapping that would leave `LocalMediaRoot` is empty. A Windows path (a drive letter, or a UNC share written with backslashes) has its backslashes turned into slashes, matches the root without regard to case, and without a root loses its volume.
- `max-clip-dur` and `session-poll-sec` are second counts. An environment value arrives as a string and still means seconds.
- The clip queue keeps its own copy of every job and hands workers snapshots. Waiting jobs are ids in submission order, so `Submit` and `Requeue` never block. Status and progress reach the database through the queue's status callback, which reports one change at a time and always writes the latest state.
- Persisted clips reload before the server listens, unfinished ones oldest first, so every request sees every clip and a delete cannot be undone by a late reload.
- `num-workers` is clamped to at least 1 when configuration loads.
- Clip edits go through `catalog.Update` or `catalog.UpdateAndRegenerate`, which change the queue's entry and save it through the queue's ordered report path, so every read and the next start see the edit. A rendering clip only takes a change `clip.RendersLike` calls equal, such as a rename. A type change renders the clip again, and a finished render removes the clip's files for the other types. JSON edits decode into `clip.EditRequest`, where an omitted field keeps the stored value. Clip cards post hidden copies of settings they do not show. Every clip card, on the media page, on `/clips`, and swapped in after a save, is filled from its own probed source through `view.ApplySource`, so an HDR source always offers the HDR checkbox. `/clips` probes its sources through `MediaSource.DescribePaths`, four at a time, on top of the probe cache.
- Auth is single-owner. The first Plex account to sign in claims the `users` row with role `owner`. When `plex_tokens` holds a token, only the account behind the newest one may claim. `identity.Auth.SignIn` refuses every other account and never stores or binds a refused or invalid token. Sessions only gain a token through sign-in. The session carries the token and `plex_user_id`, and `middleware.AuthGuard` resolves that id against `users` on every request, so deleting the row (`outtake owner reset`) revokes every session. Logout ends one session. **Forget server** clears the binding. The binding stores the server's machine id, and every sign-in replaces the bound server's token with the one plex.tv reports for that machine, keeping the bound connection. A binding stored without a machine id is identified through its `/identity` first. A PMS 401 wraps `plex.ErrUnauthorized`. The session monitor warns once per distinct failure, and the dashboard and servers page show a notice while the bound server refuses its token. Keep `users` and its roles forward-compatible with a later multi-user model.
- `middleware.HostGuard` answers 421 to a Host that is not an IP literal, localhost, a dotless or private-suffix name, the public base URL's host, or listed in `allowed-hosts`. Tests that drive the router through httptest (Host `example.com`) set `AllowedHosts: "example.com"`.
- Pages send `Cross-Origin-Opener-Policy: same-origin-allow-popups`, and the Plex callback page (`/api/auth/callback`) sends `unsafe-none`, so the sign-in popup keeps its opener on the way back from Plex, which sends none. A stricter value on either page cuts the popup off, which a browser test confirmed. `login.js` polls `/api/auth/status` only once the popup is open, and stops when the PIN's `expiresIn` lapses.
- The playback panel reads the session monitor's cache. Its mark buttons read `/media/item/:id/position` when clicked, which asks the bound server's `/status/sessions` directly through `identity.Auth.LiveSessions` within 3 seconds, and fall back to the panel's offset. A paused client's position is exact. A playing one is as old as the client's last report to Plex, so the panel advises pausing.
- `app.js` sets up each export form once (`data-export-ready`), so a swap elsewhere, such as the playback refresh, never touches it. A submit holds every export button on its form (`data-submitting`) until the page is shown again or its htmx request ends.
- The e2e environment skips the auth guard, so `app.New` refuses it on a non-loopback listen address.
- Logging is configured once in `cmd/server` (and the e2e harness), not in `app.New`. Tests build many apps in parallel, and rewriting zerolog's globals while another app logs is a data race.
- `preview.Service.Close` cancels running previews and waits for them, and `App.Close` calls it before the database closes. A completed preview that retention evicts has its file deleted.
- Sessions persist in the `sessions` table through `database.SessionStore`, and CSRF tokens live in the session, so both survive a restart. `app` sweeps expired sessions. `/assets` (including a missing asset, which answers 404) and `/api/healthz` are mounted before the session middleware, so they never create a session.
- The Fiber app runs with `Immutable: true`, so strings read off a request (`Params`, `FormValue`, `Query`) are copies a handler may keep. It also sets a read and an idle timeout, and no write timeout, because downloads and ranged video stream for as long as they need.
- Static files are embedded by `internal/web/assets`. Pages link them through `assets.URL`, which adds a `?v=` content hash. A request carrying the current hash is cached for a year, and any other is served `no-cache`. Text assets are compressed. Never hard-code an `/assets/` URL in a template.
- Clip files, downloads, and previews go through `respond.SendRangedFile`, which opens the file on every request, skipping Fiber's handle cache, and sends `Cache-Control: no-cache`. A regenerate moves a new file into place under the same path, so clip cards link the file as `/clips/<id>/file?v=<version>`, versioned by the clip's `UpdatedAt`, which every finished render advances. Chromium reuses media it loaded for a URL within a page without revalidating.
- Library lists are cached per server for a minute in `library.LibraryCache`, which the sidebar and every browse fragment share. Callers that miss together share one request, and a slower, older answer never replaces a newer one. A failed request is not cached.
- The media page reads an item's metadata once. `GetMediaItem` carries the file path in `MediaItem.FilePath` (never serialized), and the page probes that path rather than resolving the id again.
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
