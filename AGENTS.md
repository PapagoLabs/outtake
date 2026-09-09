# Outtake

Plex clip manager. Go 1.27, Fiber v3, templ, HTMX, Cobra. App code lives under `internal/` (no `pkg/`). Composition root: `internal/app`. CLI: `internal/cli`. Clip queue/storage/worker under `internal/clip`. Version metadata: `internal/metadata`. Entrypoint: `main.go`.

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

Validate with `task lint-ci` then `task vet`, not `go build`. Single package: `go test ./internal/media -run TestFoo` after `task templ`.

Server CLI is `outtake server start` (or `go run . server start`), not `serve`. Default listen `:8080`. Do not add a templ `:8080`→`:8090` proxy or a `docker-compose.dev.yml` overlay; one `docker-compose.yml` plus `.env`.

## Generated code

`**/*.templ.go` is gitignored. `task templ` is `templ generate` then `goimports -local github.com/PapagoLabs/outtake -w ./internal/web`. Ungrouped imports after generate are fixed by that goimports pass, not by skipping generate.

Do not add templ/goimports hooks to GoReleaser. CI and `task goreleaser*` already generate first. Do not edit Mockery output; regenerate with `task mock`.

CSS: `task tailwind` reads `internal/web/assets/css/input.css`.

## Nested module and e2e

`scripts/download-ffmpeg` is its own module (`outtake-scripts`). Test it with `go test -v` in that directory. Security CI scans it separately.

E2E is `//go:build e2e` under `testing/e2e` and is **not** in `go test ./...`. Needs ffmpeg plus `testing/e2e/.env` (from `.env.example`). `task test-e2e`. White-box tests sit beside the code in the same package. There is no `testing/integration` tree.

## Layout

- Dockerfiles: `build/docker/Dockerfile` (GoReleaser image context) and `Dockerfile.dev` (source build used by compose).
- GoReleaser: `build/goreleaser/stable.yaml` (git tag `vX.Y.Z`) and `nightly.yaml`. Docker `hooks.pre` runs `scripts/download-ffmpeg` into the image context. Ship ffmpeg/ffprobe binaries, not the downloader script.
- Images: `papagolabs/outtake` and `ghcr.io/papagolabs/outtake`.
- Schema is greenfield/squashed: `internal/database/migrations/001_initial.sql` and `postgres/001_initial.sql`.
- IDs: Go stdlib `uuid`, not `github.com/google/uuid`.
- `References/` is local-only (gitignored).

## CI

Workflows call templ, goimports, and goreleaser directly, not Taskfile. Go lint is `.github/workflows/lint-go.yaml` (no `lint.yaml`). lint-go / test / vet / security are pull_request + path filters, not push. `lint-gh.yaml` only on `.github/workflows/**`. Stable release: exact `vX.Y.Z` tags, `cancel-in-progress: false`. Changelog: git-cliff via `update-changelog.yaml` on `main`.

## Domain

- Timecode is `internal/media/timecode` (FromSeconds / Parse / String) over Clock/FFmpegClock. Parse through milliseconds. Library duration is HH:MM:SS; clip editing is HH:MM:SS.mmm.
- Black-bar trim uses `cropdetect=limit=24/255`. A bare `24` is 24/65535 on FFmpeg 9 10-bit HDR and misses letterboxing.
- Optional web-safe color (off by default) tone-maps HDR on CPU with `zscale`+`tonemap=hable`, using luma measured from the clip—not disc MaxCLL. Leave it off when the user will grade the file themselves. Do not use libplacebo (Vulkan/GPU).
- Export max resolution is a clip-profile setting (720p, 1080p, 1440p, 4K). Defaults: Low 720p, Medium 1080p, High 4K. Preview stays 720p.
