<p align="center">
  <img src="assets/brand/mascot.png" alt="Outtake" width="180"/>
</p>

# Outtake

Outtake is a web app that cuts video clips, GIFs, and screenshots from your Plex libraries. Sign in with Plex, pick a title, mark a range, and save the export.

## What it does

- Sign in with a Plex PIN (or paste a token).
- Browse libraries, search titles, and open an item.
- Mark start and end from live Plex playback, or type the times yourself.
- Export a video clip, GIF, or screenshot, then preview and download it.
- Optional clip profiles and a **Trim black bars** option on each export.

The server listens on port 8080 by default.

## Requirements

- A Plex account and a Plex Media Server that Outtake can reach.
- Read access to the media files Plex reports (on the host, or mounted into Docker).
- **Docker images** ship static `ffmpeg` and `ffprobe`.
- A **host binary** needs `ffmpeg` and `ffprobe` on `PATH` (or set `OUTTAKE_FFMPEG_PATH` and `OUTTAKE_FFPROBE_PATH`).

There are no GitHub Releases yet, so Docker Compose is the supported way to run Outtake.

## Install

### Docker Compose (published image)

Images are `ghcr.io/papagolabs/outtake` and `papagolabs/outtake`. The example compose file pulls GHCR and maps your library read-only to `/media`:

```bash
export OUTTAKE_MEDIA_PATH=/path/to/your/media
docker compose -f examples/docker-compose.yaml up -d
```

Then open [http://localhost:8080](http://localhost:8080).

Set `OUTTAKE_MEDIA_PATH` to the directory that contains the files Plex plays. If Plex reports a different filesystem prefix than that mount, also set `OUTTAKE_PLEX_MEDIA_ROOT` (see [Configuration](#configuration)).

### Docker Compose (build locally)

From the repository root, copy [`.env.example`](.env.example) to `.env`, point `OUTTAKE_MEDIA_PATH` at your library, and build:

```bash
cp .env.example .env
docker compose up --build
```

This uses `build/docker/Dockerfile.dev` and also bundles `ffmpeg` and `ffprobe`.

## Configuration

Outtake reads `OUTTAKE_*` environment variables. Compose files also use `OUTTAKE_MEDIA_PATH` for the host library bind (that name is not an application setting).

Docker images store the database and exports under `/data` (`outtake.db` and `output/`). The example compose file keeps that volume as `outtake-data`.

| Variable | Purpose | Default |
| --- | --- | --- |
| `OUTTAKE_LISTEN_ADDR` | Address the web server binds | `0.0.0.0:8080` (images use `:8080`) |
| `OUTTAKE_PUBLIC_BASE_URL` | URL Plex should return to after PIN login | derived from the listen address, or `http://localhost:8080` in images |
| `OUTTAKE_LOCAL_MEDIA_ROOT` | Local directory that should contain Plex files (container mount is usually `/media`) | unset (compose examples set `/media`) |
| `OUTTAKE_PLEX_MEDIA_ROOT` | Prefix Plex reports for those files; replaced by `OUTTAKE_LOCAL_MEDIA_ROOT` | unset |
| `OUTTAKE_FFMPEG_PATH` | `ffmpeg` binary | `ffmpeg` (images use `/usr/bin/ffmpeg`) |
| `OUTTAKE_FFPROBE_PATH` | `ffprobe` binary | `ffprobe` (images use `/usr/bin/ffprobe`) |
| `OUTTAKE_MAX_CLIP_DUR_SEC` | Maximum export duration in seconds | `600` |
| `OUTTAKE_CROP_BLACK_BARS` | Default for **Trim black bars** | `false` |
| `OUTTAKE_LOG_LEVEL` | `debug`, `info`, `warn`, or `error` | `info` |
| `OUTTAKE_PLEX_SERVER_URL` | Optional Plex Media Server URL | unset |
| `OUTTAKE_PLEX_TOKEN` | Optional Plex token (PIN login does not need this) | unset |

Plex gives Outtake absolute file paths. If those paths are not readable as-is (typical in Docker), set `OUTTAKE_LOCAL_MEDIA_ROOT` to the mount. When `OUTTAKE_PLEX_MEDIA_ROOT` is also set, that prefix is stripped and the remainder is joined under the local root. When only the local root is set, the Plex path is joined under that mount.

If you open Outtake from another host, set `OUTTAKE_PUBLIC_BASE_URL` to the URL you type in the browser so the Plex PIN callback can return.

## First run

1. Open [http://localhost:8080](http://localhost:8080). Unauthenticated visits redirect to **Login**.
2. Choose **Sign in with Plex**. Outtake opens the Plex Auth App in a popup and shows **Waiting for Plex authorization...** until you approve it.
3. Or paste a token into **Plex Token** and choose **Sign In**.
4. If Outtake finds exactly one Media Server, it uses that server and continues to the dashboard. Otherwise it opens **Select a Plex server**. Choose **Use this server**, or enter a custom URL such as `https://plex.example.com` and choose **Use this URL**.

You can change servers later under **Settings → Servers**.

## Make a clip

1. Open **Media Libraries** (or **Browse Media** on the dashboard).
2. Search, or browse a library, then **Open** a title. Folders and shows use **Browse** until you reach a playable item.
3. On the item page, play the title in Plex if you want live markers. When Plex is playing, use **Set start from Plex** and **Set end from Plex**. You can also type **Start** and **End** yourself.
4. Under **New export**, set **Export as** to **Video clip**, **GIF**, or **Screenshot**, pick a **Profile**, and optionally **Trim black bars**.
5. Choose **Preview** to check the segment, then **Save clip**.

If something is already playing, the dashboard **Live Sessions** list includes **Clip now**.

Finished exports appear on the item and on **Clips**. When a job is **completed** and the file is on disk, use **Download**. Progress updates while a job is pending or processing.

## Troubleshooting

- **Login never finishes.** Approve the Plex popup. If you reach Outtake through a hostname other than localhost, set `OUTTAKE_PUBLIC_BASE_URL` to that URL.
- **No Plex servers were discovered.** Use **Custom server URL** on the servers page. Outtake must be able to reach that address.
- **No media found.** Select a server first, then search or browse again.
- **Clips fail or files are missing.** The path Plex reports must be readable. In Docker, mount the library and set `OUTTAKE_PLEX_MEDIA_ROOT` / `OUTTAKE_LOCAL_MEDIA_ROOT` so that path lands on `/media`.
- **ffmpeg / ffprobe errors on a host binary.** Install both tools and keep them on `PATH`, or set the path variables above. Docker images already include them.

## License

Outtake is licensed under the [GNU Affero General Public License v3.0](LICENSE).

## Contributing

Development commands and repo conventions live in [AGENTS.md](AGENTS.md).
