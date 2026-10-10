---
title: Configuration
description: Every OUTTAKE_ environment variable, how media paths are mapped, and which host names Outtake answers to
weight: 2
---

Outtake reads its settings from `OUTTAKE_*` environment variables. There is no settings file. The command line has a single flag, `--listen`, which overrides `OUTTAKE_LISTEN_ADDR` (see the [CLI reference](/cli-reference/)).

The Compose files also use `OUTTAKE_MEDIA_PATH` and `OUTTAKE_DATA_PATH` to choose what to mount into the container. Those two are read by Docker Compose, not by Outtake.

## Variables

### Server

| Variable | Purpose | Default |
| --- | --- | --- |
| `OUTTAKE_LISTEN_ADDR` | Address the web server binds | `0.0.0.0:8080` (images use `:8080`) |
| `OUTTAKE_PUBLIC_BASE_URL` | The URL you open Outtake at. Plex returns there after login | derived from the listen address, or `http://localhost:8080` in images |
| `OUTTAKE_ALLOWED_HOSTS` | More host names Outtake answers to, separated by commas. A leading dot covers a whole domain (`.example.com`), and `*` turns the host check off | unset |
| `OUTTAKE_LOG_LEVEL` | `debug`, `info`, `warn`, or `error` | `info` |
| `OUTTAKE_ENV` | `production` writes JSON logs, and `development` writes readable console logs. `e2e` is for the test suite only: it skips login, so Outtake refuses it on any address but loopback | `production` |

### Plex

| Variable | Purpose | Default |
| --- | --- | --- |
| `OUTTAKE_LOCAL_MEDIA_ROOT` | Directory where Outtake can read the files Plex plays, such as the `/media` mount in a container | unset (the Compose examples set `/media`) |
| `OUTTAKE_PLEX_MEDIA_ROOT` | Prefix Plex reports for those files, replaced by `OUTTAKE_LOCAL_MEDIA_ROOT` | unset |
| `OUTTAKE_SESSION_POLL_SEC` | How often to read live Plex playback, in seconds | `10` |
| `OUTTAKE_PLEX_SERVER_URL` | Plex Media Server URL to use at every start, in place of the server chosen under **Servers**. Takes effect only with `OUTTAKE_PLEX_TOKEN` | unset |
| `OUTTAKE_PLEX_TOKEN` | Token Outtake sends to that server. Browser login does not need it | unset |
| `OUTTAKE_PLEX_CLIENT_ID` | Client identifier Outtake shows to Plex | generated on first start and kept in the database |

### Clips and FFmpeg

| Variable | Purpose | Default |
| --- | --- | --- |
| `OUTTAKE_MAX_CLIP_DUR` | Longest clip, in seconds | `600` |
| `OUTTAKE_CROP_BLACK_BARS` | Whether **Trim black bars** starts checked | `false` |
| `OUTTAKE_NUM_WORKERS` | How many clips render at once. A value below `1` means `1` | `2` |
| `OUTTAKE_MAX_CONCURRENT_PREVIEWS` | How many previews render at once. A value below `1` means `1` | `2` |
| `OUTTAKE_FFMPEG_PATH` | `ffmpeg` binary | `ffmpeg` (images use `/usr/bin/ffmpeg`) |
| `OUTTAKE_FFPROBE_PATH` | `ffprobe` binary | `ffprobe` (images use `/usr/bin/ffprobe`) |
| `OUTTAKE_FFMPEG_TIMEOUT_SEC` | Longest one FFmpeg run may take, in seconds. `0` allows 20 times the clip's length, or 60 times for the HEVC encode of a clip that keeps HDR, and never less than 30 minutes | `0` |

### Storage

These are covered in more detail under [Storage and Databases](/setup/storage/).

| Variable | Purpose | Default |
| --- | --- | --- |
| `OUTTAKE_DATABASE_BACKEND` | `sqlite` or `postgres` | `sqlite` |
| `OUTTAKE_DATABASE_PATH` | SQLite database file | `~/.local/share/outtake/outtake.db` (images use `/data/outtake.db`) |
| `OUTTAKE_DATABASE_URL` | Postgres connection string, when the backend is `postgres` | unset |
| `OUTTAKE_STORAGE_BACKEND` | `filesystem` or `s3` | `filesystem` |
| `OUTTAKE_STORAGE_PATH` | Where clips are written, or the local scratch directory with S3 | `~/.local/share/outtake/output` (images use `/data/output`) |
| `OUTTAKE_S3_ENDPOINT` | S3-compatible API endpoint | unset |
| `OUTTAKE_S3_BUCKET` | Bucket for clips | unset |
| `OUTTAKE_S3_REGION` | Region | `us-east-1` |
| `OUTTAKE_S3_ACCESS_KEY` | Access key | unset |
| `OUTTAKE_S3_SECRET_KEY` | Secret key | unset |
| `OUTTAKE_S3_USE_PATH_STYLE` | Path-style URLs, which SeaweedFS and RustFS expect | `true` |

## Media paths

Plex tells Outtake the absolute path of each file it plays, and Outtake reads that file itself. On the same machine as Plex the path usually works as it is. In a container it rarely does, because the library is mounted somewhere else, such as `/media`.

- Set `OUTTAKE_LOCAL_MEDIA_ROOT` to the mount.
- If Plex reports a prefix of its own, such as `/data/movies/Film.mkv` for a library you mounted at `/media`, set `OUTTAKE_PLEX_MEDIA_ROOT=/data`. Outtake replaces that prefix with the local root and reads `/media/movies/Film.mkv`.
- With only the local root set, the whole Plex path is joined under it.

The prefix only matches whole directories, so `/data/media` never claims `/data/media-other`, and a path that would leave the local root is refused. Paths from a Windows Plex server, with a drive letter or a UNC share, are mapped too: their backslashes become slashes and the prefix matches without regard to case.

These two settings are about the source media only. Clips are stored under `OUTTAKE_STORAGE_PATH` or in S3, never in the media mount.

## Opening Outtake from another machine

Set `OUTTAKE_PUBLIC_BASE_URL` to the URL you type into the browser, such as `https://outtake.example.com`. Plex sends you back to it after login, so a wrong value leaves login waiting.

Outtake answers only requests addressed to:

- an IP address or `localhost`
- a name without dots, such as `nas`
- a private name, such as `nas.local` or a name under `.lan`, `.home`, `.home.arpa`, or `.internal`
- the host in `OUTTAKE_PUBLIC_BASE_URL`
- a host listed in `OUTTAKE_ALLOWED_HOSTS`

Any other host gets `421 Misdirected Request`. That stops a web page on another site from reaching your Outtake through DNS rebinding. Add each extra name you use to `OUTTAKE_ALLOWED_HOSTS`.
