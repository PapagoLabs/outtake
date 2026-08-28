# AGENTS

## Generate

`*_templ.go` is gitignored. After clone or any `.templ` edit, run `task templ` before compile or test. Tailwind: edit `internal/web/assets/css/input.css`, then `task tailwind`. Mocks: `task mock`. Edit `.templ` sources and interfaces; leave `*_templ.go` and `**/mocks/` to the generators. Never hand-edit Mockery output.

## Lint

Config: `build/golangci-lint/golangci-lint.yaml`.
Command: `task lint`.
`task lint-ci` runs without `--fix`. PRs must pass lint without `--fix`.
SPDX `AGPL-3.0-or-later` plus copyright on every Go file (goheader).
Viper/mapstructure keys are kebab-case (`listen-addr`). Env is `OUTTAKE_` with underscores. JSON tags are camelCase (`mediaId`).

## Fiber

Web and API use Fiber v3. HTML is Templ + HTMX. Pages live in `internal/web/pages/`; handlers in `internal/web/handlers/`; routes are mounted in `internal/app/app.go`.

## Validate

`task vet`, then `task lint`, then `task test`. Use `go vet` rather than `go build` for compiler checks. E2E (`task test-e2e`) needs FFmpeg and Plex credentials in `testing/e2e/.env`.
Current PapagoLabs Go minor is 1.27. Read `go.mod` for this module (currently 1.26.6).

## Layout

Entry: `main.go`. Internal code lives in `internal/`. Do not require `pkg/`. White-box tests sit beside the package. E2E tests: `testing/e2e`. CLI: `outtake server start`, `outtake health`, `outtake version`.

## Commits

Conventional Commits, Angular type and scope, imperative mood. First line under 72 characters. Body bullets, most important first, max 5. Do not use AdGuard `pkg:` headers.

## Pull requests

Title: release-note style (scope optional). Body: 2-3 sentence summary, then Problem, Solution, and Changes headings.

## Comments

Natural English sentences with a period. Comment above the entity. Function docs include Parameters and Returns lists.

## Docker

Build context is always the repo root.

- Local/from-source image: `build/docker/Dockerfile.dev` (`docker-compose.yml` uses this).
- Release image: `build/docker/Dockerfile` is a GoReleaser `dockers_v2` context (pre-built binary at `${TARGETPLATFORM}/outtake`). Do not `docker build` it from the repo root.

`docker-compose.dev.yml` is gitignored machine-local overlay. Keep host media paths out of tracked compose files.
