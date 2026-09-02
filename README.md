<p align="center">
  <img src="assets/brand/mascot.png" alt="Outtake" width="180"/>
</p>

# Outtake

A clip manager for Plex libraries. Create video clips, GIFs, and screenshots from media on your Plex server in a web UI.

## Requirements

- A running [Plex Media Server](https://www.plex.tv/) and a Plex account
- Access to the same media files Plex uses (or a path mapping onto a local/container mount)
- [FFmpeg](https://ffmpeg.org/) and ffprobe — bundled in the Docker image; required on `PATH` if you run a host binary

## Install

### Docker Compose (recommended)

No GitHub Releases are published yet. The supported way to run Outtake is the published image:

```bash
# point this at the host directory Plex uses for media
export OUTTAKE_MEDIA_PATH=/path/to/your/media
export OUTTAKE_PUBLIC_BASE_URL=http://localhost:8080

docker compose -f examples/docker-compose.yaml up -d
```

The example compose file pulls `ghcr.io/papagolabs/outtake:latest`, publishes port `8080`, stores data in a volume, and mounts media read-only at `/media`.

If Plex reports file paths that do not match that mount, set `OUTTAKE_PLEX_MEDIA_ROOT` (the prefix Plex uses) and `OUTTAKE_LOCAL_MEDIA_ROOT` (usually `/media`).

### Build a local image

From the repo root (uses `build/docker/Dockerfile.dev`, which compiles Outtake and bundles ffmpeg/ffprobe):

```bash
cp .env.example .env   # then set OUTTAKE_MEDIA_PATH and OUTTAKE_PUBLIC_BASE_URL
docker compose up --build
```

Open [http://localhost:8080](http://localhost:8080).

## First run

1. Open the web UI.
2. Choose **Sign in with Plex**. Outtake starts a Plex PIN login and opens a Plex authorization window. Wait until the page shows that you are authenticated.
3. Or paste a Plex token into the form and sign in that way.
4. If you have more than one Plex server, pick one under **Settings → Servers**.

If sign-in never completes, set `OUTTAKE_PUBLIC_BASE_URL` to the URL you actually use in the browser (not `0.0.0.0`).

## Make a clip

1. Go to **Media Libraries** and open a title (or start from a live session on the dashboard).
2. Optionally play the item in Plex and use **Set start from Plex** / **Set end from Plex** so Outtake reads the current playback position.
3. Set start time and duration (default max is 10 minutes).
4. Choose type: video clip, GIF, or screenshot, plus a quality profile.
5. Create the job. Progress shows on the dashboard and under **Clips**.

Outtake reads media from disk (after any path remap) and encodes with ffmpeg in the background.

## Configuration

Settings are environment variables with an `OUTTAKE_` prefix. Defaults below are for a host process; the Docker image overrides data and ffmpeg paths to `/data` and `/usr/bin`.

| Variable | What it does | Default |
| --- | --- | --- |
| `OUTTAKE_LISTEN_ADDR` | HTTP listen address | `0.0.0.0:8080` |
| `OUTTAKE_PUBLIC_BASE_URL` | Public URL for Plex PIN callbacks | derived from `OUTTAKE_LISTEN_ADDR` |
| `OUTTAKE_DATABASE_PATH` | SQLite database | `~/.local/share/outtake/outtake.db` |
| `OUTTAKE_STORAGE_PATH` | Clip/GIF/screenshot output | `~/.local/share/outtake/output` |
| `OUTTAKE_FFMPEG_PATH` | ffmpeg binary | `ffmpeg` |
| `OUTTAKE_FFPROBE_PATH` | ffprobe binary | `ffprobe` |
| `OUTTAKE_LOG_LEVEL` | `debug`, `info`, `warn`, or `error` | `info` |
| `OUTTAKE_ENV` | `production` or `development` | `production` |
| `OUTTAKE_PLEX_SERVER_URL` | Optional Plex server URL | unset |
| `OUTTAKE_PLEX_TOKEN` | Optional Plex token | unset |
| `OUTTAKE_PLEX_CLIENT_ID` | Plex client id | generated if unset |
| `OUTTAKE_PLEX_MEDIA_ROOT` | Plex filesystem prefix to strip | unset |
| `OUTTAKE_LOCAL_MEDIA_ROOT` | Local/container prefix that replaces it | unset |
| `OUTTAKE_SESSION_POLL_SEC` | How often to poll Plex sessions | `10` |
| `OUTTAKE_NUM_WORKERS` | Background encode workers | `2` |
| `OUTTAKE_MAX_CLIP_DUR_SEC` | Maximum clip duration (seconds) | `600` |
| `OUTTAKE_CROP_BLACK_BARS` | Default letterbox/pillarbox crop | `false` |

On a host install, data lives under XDG (`XDG_DATA_HOME`, default `~/.local/share/outtake/`). In Docker, persist `/data`.

Compose-only volume helpers (not app settings): `OUTTAKE_MEDIA_PATH` (host media directory) and `OUTTAKE_DATA_PATH` (optional host data directory for the local compose file).

## Troubleshooting

- **Plex sign-in never finishes.** Set `OUTTAKE_PUBLIC_BASE_URL` to the browser URL, allow the popup, and complete authorization on plex.tv.
- **Jobs fail immediately / cannot open media.** Outtake must read the file Plex reports. Mount that library and set `OUTTAKE_PLEX_MEDIA_ROOT` / `OUTTAKE_LOCAL_MEDIA_ROOT` if the prefixes differ.
- **ffmpeg not found** (host binary only). Install ffmpeg/ffprobe or set `OUTTAKE_FFMPEG_PATH` and `OUTTAKE_FFPROBE_PATH`. Docker images already bundle them.
- **Nothing listens on 8080.** Check `OUTTAKE_LISTEN_ADDR` and that port `8080` is published.

## License

[GNU Affero General Public License v3.0](LICENSE)

## Developing

Contributor workflow lives in [AGENTS.md](AGENTS.md). The module is Go 1.27.1. Generated Templ is not committed (`task templ` after clone). CLI: `outtake server start`, `outtake health`, `outtake version`.
