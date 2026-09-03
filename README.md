<!-- markdownlint-disable -->
<div align="center">
  <a href="https://github.com/PapagoLabs/outtake">
    <img src="assets/brand/mascot.png" alt="Outtake" width="180" />
  </a>

# Outtake

  A web app that cuts video clips, GIFs, and screenshots from your Plex libraries.<br/><br/>

  [![Go 1.27+](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://go.dev/)
  [![Test](https://github.com/PapagoLabs/outtake/actions/workflows/test.yaml/badge.svg?branch=main)](https://github.com/PapagoLabs/outtake/actions/workflows/test.yaml)
  [![Lint](https://github.com/PapagoLabs/outtake/actions/workflows/lint-go.yaml/badge.svg?branch=main)](https://github.com/PapagoLabs/outtake/actions/workflows/lint-go.yaml)
  [![Docker Hub pulls](https://img.shields.io/docker/pulls/papagolabs/outtake.svg)](https://hub.docker.com/r/papagolabs/outtake)
  [![GHCR](https://img.shields.io/badge/ghcr.io-papagolabs%2Fouttake-blue?logo=github)](https://github.com/PapagoLabs/outtake/pkgs/container/outtake)
  [![License](https://img.shields.io/github/license/PapagoLabs/outtake.svg)](LICENSE)
</div>
<!-- markdownlint-restore -->

## Table of Contents

- [What it does](#what-it-does)
- [Requirements](#requirements)
- [Install](#install)
  - [Docker Compose](#docker-compose)
  - [Kubernetes / Helm](#kubernetes--helm)
- [Configuration](#configuration)
- [First run](#first-run)
- [Make a clip](#make-a-clip)
- [Troubleshooting](#troubleshooting)
- [License](#license)
- [Contributing](#contributing)

## What it does

- Sign in with Plex (or paste a token).
- Browse libraries, search titles, and open an item.
- Mark start and end from live Plex playback, or type the times yourself.
- Export a video clip, GIF, or screenshot, then preview and download it.
- Manage named clip profiles (CRF, encoder preset, audio bitrate, max
  resolution) and optionally **Trim black bars** on each export.

The server listens on port 8080 by default.

## Requirements

- A Plex account and a Plex Media Server that Outtake can reach.
- Read access to the media files Plex reports (on the host, or mounted
  into Docker).
- **Docker images** ship static `ffmpeg` and `ffprobe`.
- A **host binary** needs `ffmpeg` and `ffprobe` on `PATH` (or set
  `OUTTAKE_FFMPEG_PATH` and `OUTTAKE_FFPROBE_PATH`).

## Install

Two deploy modes: **Docker Compose** is the local filesystem + SQLite
path. **Kubernetes / Helm** is for clusters. You point NFS (or an
existing claim) at Plex media. Clips and metadata do not live on that
share.

### Docker Compose

Images are published as `ghcr.io/papagolabs/outtake` and
`papagolabs/outtake`. This matches [`examples/docker-compose.yaml`](examples/docker-compose.yaml):

```yaml
services:
  outtake:
    image: ghcr.io/papagolabs/outtake:latest
    container_name: outtake
    ports:
      - "8080:8080"
    volumes:
      - outtake-data:/data
      - ${OUTTAKE_MEDIA_PATH:-/path/to/your/media}:/media:ro
    environment:
      OUTTAKE_LOG_LEVEL: info
      OUTTAKE_LISTEN_ADDR: 0.0.0.0:8080
      OUTTAKE_PUBLIC_BASE_URL: ${OUTTAKE_PUBLIC_BASE_URL:-http://localhost:8080}
      OUTTAKE_PLEX_MEDIA_ROOT: ${OUTTAKE_PLEX_MEDIA_ROOT:-}
      OUTTAKE_LOCAL_MEDIA_ROOT: ${OUTTAKE_LOCAL_MEDIA_ROOT:-/media}
    restart: unless-stopped

volumes:
  outtake-data:
```

Point `OUTTAKE_MEDIA_PATH` at the directory that contains the files Plex
plays, then start the stack:

```bash
export OUTTAKE_MEDIA_PATH=/path/to/your/media
docker compose up -d
```

Then open [http://localhost:8080](http://localhost:8080).

Compose defaults to filesystem blobs and SQLite. Clips land on
`OUTTAKE_STORAGE_PATH` (the `/data` volume), not on the media bind.
[`docker-compose.yml`](docker-compose.yml) is the same local path when you
build from source.

If Plex reports a different filesystem prefix than that mount, also set
`OUTTAKE_PLEX_MEDIA_ROOT` (see [Configuration](#configuration)).

### Docker

```bash
docker run -d \
  --name outtake \
  -p 8080:8080 \
  -v outtake-data:/data \
  -v /path/to/your/media:/media:ro \
  -e OUTTAKE_LOCAL_MEDIA_ROOT=/media \
  -e OUTTAKE_PUBLIC_BASE_URL=http://localhost:8080 \
  ghcr.io/papagolabs/outtake:latest
```

The image entrypoint is `/outtake`. The default command is `server start`.

### Binary

Tagged [GitHub Releases](https://github.com/PapagoLabs/outtake/releases)
are not published yet. When they are, unpack the `outtake` archive for
your OS, install ffmpeg and ffprobe, and run:

```bash
./outtake server start
```

The server binds `0.0.0.0:8080` by default. Override with `--listen` or
`OUTTAKE_LISTEN_ADDR`. On Linux, the database and exports default to
`~/.local/share/outtake/`.

Until a release exists, use Docker or Compose.

### Local image from source

From a repository checkout, copy [`.env.example`](.env.example) to `.env`,
set `OUTTAKE_MEDIA_PATH`, and build:

```bash
cp .env.example .env
docker compose up --build
```

This uses `build/docker/Dockerfile.dev` and also bundles ffmpeg and
ffprobe.

### Kubernetes / Helm

The chart lives at
[`deploy/helm/outtake`](https://github.com/PapagoLabs/outtake/tree/main/deploy/helm/outtake).
There is no Helm repo or OCI chart. Install it from that GitHub tree.
Do **not** clone the repository. Helm pulls Git with
[helm-git](https://github.com/aslafy-z/helm-git):

```bash
helm plugin install https://github.com/aslafy-z/helm-git
helm repo add papagolabs \
  --username "$GITHUB_USERNAME" \
  --password "$GITHUB_TOKEN" \
  git+https://github.com/PapagoLabs/outtake@deploy/helm?ref=main
```

The GitHub repository is private. On Helm 3.14+, `--username` is your
GitHub username and `--password` is a token that can read the repo. Or
use SSH:

```bash
helm repo add papagolabs \
  git+ssh://git@github.com/PapagoLabs/outtake@deploy/helm?ref=main
```

Values and optional backends are in the chart
[README](https://github.com/PapagoLabs/outtake/blob/main/deploy/helm/outtake/README.md).

The only site input is Plex media: set `media.nfs.server` and
`media.nfs.path`, or `media.existingClaim`. Clip blobs stay on
`storage-path` or S3. They do **not** live on the media NFS share.

Defaults match Compose: `outtake.storageBackend` is `filesystem` and
`outtake.databaseBackend` is `sqlite`. The image is
`ghcr.io/papagolabs/outtake:latest` (no tagged release yet).

Minimal install (NFS for Plex media, filesystem and SQLite for clips):

```bash
helm install outtake papagolabs/outtake \
  --set media.nfs.server=nfs.example.internal \
  --set media.nfs.path=/export/plex
```

Optional in-cluster backends (at most one blob store and one database):
SeaweedFS or RustFS, and CockroachDB (`backends.cockroach`) or
CloudNativePG (`backends.cnpg`, and `backends.cnpgOperator` if you also
need the operator). Enabling SeaweedFS or RustFS selects S3 storage.
Enabling Cockroach or CNPG selects postgres. Set those
`outtake.storageBackend` / `outtake.databaseBackend` values yourself if
you are not using a chart backend. A working combo is SeaweedFS +
Cockroach (full example:
[`values-distributed.yaml`](https://github.com/PapagoLabs/outtake/blob/main/deploy/helm/outtake/examples/values-distributed.yaml)).
S3 access keys are still required. Point `media.nfs` (or
`media.existingClaim`) at your Plex library:

```bash
helm install outtake papagolabs/outtake \
  --set backends.seaweedfs.enabled=true \
  --set backends.cockroach.enabled=true \
  --set outtake.s3.accessKey=seaweedfs \
  --set outtake.s3.secretKey=seaweedfs \
  --set media.nfs.server=nfs.example.internal \
  --set media.nfs.path=/export/plex
```

## Configuration

Outtake reads `OUTTAKE_*` environment variables. Compose files also use
`OUTTAKE_MEDIA_PATH` for the host library bind (that name is not an
application setting).

Docker images store the SQLite database and filesystem exports under
`/data` (`outtake.db` and `output/`). The example compose file keeps that
volume as `outtake-data`. Clip blobs use `OUTTAKE_STORAGE_PATH` (or S3
when `OUTTAKE_STORAGE_BACKEND=s3`). Clip metadata lives in the database
(`OUTTAKE_DATABASE_PATH` or `OUTTAKE_DATABASE_URL`).
`OUTTAKE_LOCAL_MEDIA_ROOT` and `OUTTAKE_PLEX_MEDIA_ROOT` are **source
media only**, not the clip store, and not Kubernetes media NFS.

| Variable | Purpose | Default |
| --- | --- | --- |
| `OUTTAKE_LISTEN_ADDR` | Address the web server binds | `0.0.0.0:8080` (images use `:8080`) |
| `OUTTAKE_PUBLIC_BASE_URL` | URL Plex should return to after sign-in | derived from the listen address, or `http://localhost:8080` in images |
| `OUTTAKE_DATABASE_BACKEND` | `sqlite` or `postgres` | `sqlite` |
| `OUTTAKE_DATABASE_PATH` | SQLite database file | `~/.local/share/outtake/outtake.db` (images use `/data/outtake.db`) |
| `OUTTAKE_DATABASE_URL` | Postgres/pgx DSN (when backend is `postgres`) | unset |
| `OUTTAKE_STORAGE_BACKEND` | `filesystem` or `s3` | `filesystem` |
| `OUTTAKE_STORAGE_PATH` | Filesystem blobs, or S3 scratch, not Plex media / NFS | `~/.local/share/outtake/output` (images use `/data/output`) |
| `OUTTAKE_S3_ENDPOINT` | S3-compatible API endpoint | unset |
| `OUTTAKE_S3_BUCKET` | S3 bucket | unset |
| `OUTTAKE_S3_REGION` | S3 region | `us-east-1` |
| `OUTTAKE_S3_ACCESS_KEY` | S3 access key | unset |
| `OUTTAKE_S3_SECRET_KEY` | S3 secret key | unset |
| `OUTTAKE_S3_USE_PATH_STYLE` | Path-style S3 URLs (typical for SeaweedFS / RustFS) | `true` |
| `OUTTAKE_LOCAL_MEDIA_ROOT` | Local directory that should contain Plex files (container mount is usually `/media`). Source media only | unset (compose examples set `/media`) |
| `OUTTAKE_PLEX_MEDIA_ROOT` | Prefix Plex reports for those files, replaced by `OUTTAKE_LOCAL_MEDIA_ROOT`. Source media only | unset |
| `OUTTAKE_FFMPEG_PATH` | `ffmpeg` binary | `ffmpeg` (images use `/usr/bin/ffmpeg`) |
| `OUTTAKE_FFPROBE_PATH` | `ffprobe` binary | `ffprobe` (images use `/usr/bin/ffprobe`) |
| `OUTTAKE_MAX_CLIP_DUR_SEC` | Maximum clip duration in seconds | `600` |
| `OUTTAKE_CROP_BLACK_BARS` | Default for **Trim black bars** | `false` |
| `OUTTAKE_SESSION_POLL_SEC` | How often to poll live Plex playback | `10` |
| `OUTTAKE_NUM_WORKERS` | Background clip workers | `2` |
| `OUTTAKE_LOG_LEVEL` | `debug`, `info`, `warn`, or `error` | `info` |
| `OUTTAKE_PLEX_SERVER_URL` | Optional Plex Media Server URL | unset |
| `OUTTAKE_PLEX_TOKEN` | Optional Plex token (browser sign-in does not need this) | unset |
| `OUTTAKE_PLEX_CLIENT_ID` | Plex client identifier | generated if unset |

Plex gives Outtake absolute file paths. If those paths are not readable
as-is (typical in Docker), set `OUTTAKE_LOCAL_MEDIA_ROOT` to the mount.
When `OUTTAKE_PLEX_MEDIA_ROOT` is also set, that prefix is stripped and
the remainder is joined under the local root. When only the local root is
set, the Plex path is joined under that mount.

If you open Outtake from another host, set `OUTTAKE_PUBLIC_BASE_URL` to
the URL you type in the browser so Plex sign-in can return.

## First run

1. Open [http://localhost:8080](http://localhost:8080). Unauthenticated
   visits redirect to **Login**.
2. Choose **Sign in with Plex**. Outtake opens the Plex Auth App in a
   popup and shows **Waiting for Plex authorization...** until you approve
   it.
3. Or paste a token into **Plex Token** and choose **Sign In**.
4. If Outtake finds exactly one Media Server, it uses that server and
   continues to the dashboard. Otherwise it opens **Select a Plex server**.
   Choose **Use this server**, or enter a custom URL such as
   `https://plex.example.com` and choose **Use this URL**.

You can change servers later under **Settings → Servers**.

## Make a clip

1. Open **Media Libraries** (or **Browse Media** on the dashboard).
2. Search, or browse a library, then **Open** a title. Folders and shows
   use **Browse** until you reach a playable item.
3. On the item page, play the title in Plex if you want live markers.
   When Plex is playing, use **Set start from Plex** and **Set end from
   Plex**. You can also type **Start** and **End** yourself.
4. Under **New export**, set **Export as** to **Video clip**, **GIF**, or
   **Screenshot**, pick a **Profile**, and optionally **Trim black bars**.
5. Choose **Preview** to check the segment, then **Save clip**.

If something is already playing, the dashboard **Live Sessions** list
includes **Clip now**.

Finished exports appear on the item and on **Clips**. When a job is
**completed** and the file is on disk, use **Download**. Progress updates
while a job is pending or processing.

Named encode settings live under **Settings → Clip Profiles**. Lower CRF
is higher quality.

## Troubleshooting

- **Login never finishes.** Approve the Plex popup. If you reach Outtake
  through a hostname other than localhost, set `OUTTAKE_PUBLIC_BASE_URL`
  to that URL.
- **No Plex servers were discovered.** Use **Custom server URL** on the
  servers page. Outtake must be able to reach that address.
- **No media found.** Select a server first, then search or browse again.
- **Clips fail or files are missing.** The path Plex reports must be
  readable. In Docker, mount the library and set
  `OUTTAKE_PLEX_MEDIA_ROOT` / `OUTTAKE_LOCAL_MEDIA_ROOT` so that path
  lands on `/media`. On Kubernetes, that mount is `media.nfs` or
  `media.existingClaim` (source media only). Clip blobs stay on
  `OUTTAKE_STORAGE_PATH` or S3, not on the media NFS share. Clip metadata
  is in the database.
- **Helm cannot fetch the chart.** The GitHub repo is private. Install
  helm-git. Re-add `papagolabs` with `--username` and `--password` (Helm
  3.14+) or `git+ssh`.
- **Wrong storage or database backend.** Defaults are `filesystem` and
  `sqlite`. For S3, set `OUTTAKE_STORAGE_BACKEND=s3` plus the `OUTTAKE_S3_*`
  keys. For postgres, set `OUTTAKE_DATABASE_BACKEND=postgres` and
  `OUTTAKE_DATABASE_URL`. On Helm, enable at most one blob backend
  (`backends.seaweedfs` or `backends.rustfs`) and one database backend
  (`backends.cockroach` or `backends.cnpg`).
- **ffmpeg / ffprobe errors on a host binary.** Install both tools and
  keep them on `PATH`, or set the path variables above. Docker images
  already include them.

## License

Outtake is licensed under the
[GNU Affero General Public License v3.0](LICENSE).

## Contributing

Development commands and repo conventions live in [AGENTS.md](AGENTS.md).
