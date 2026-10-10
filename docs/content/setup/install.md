---
title: Install
description: Install Outtake with Docker Compose, Docker, a build from source, or a host binary
weight: 1
---

Outtake runs as one container next to your Plex Media Server. It needs to reach the server over the network and to read the media files Plex plays.

## What you need

- A Plex account and a Plex Media Server that Outtake can reach.
- Read access to the media files Plex reports, mounted into the container or on the host.
- Docker with Compose. The image ships `ffmpeg` and `ffprobe`, so there is nothing else to install.

## Install with Docker Compose

The images are `ghcr.io/papagolabs/outtake` and `papagolabs/outtake`. Save this as `docker-compose.yaml`. It is the same file as [`examples/docker/docker-compose.yaml`](https://github.com/PapagoLabs/outtake/blob/main/examples/docker/docker-compose.yaml):

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

Point `OUTTAKE_MEDIA_PATH` at the directory that holds the files Plex plays, then start it:

```bash
export OUTTAKE_MEDIA_PATH=/path/to/your/media
docker compose up -d
```

Open [http://localhost:8080](http://localhost:8080).

The database and the clips you make live in the `outtake-data` volume, mounted at `/data`. The media mount is read-only, and Outtake never writes to it.

If Plex reports a different path for the files than the one they have inside the container, set `OUTTAKE_PLEX_MEDIA_ROOT` too. [Media paths](/setup/configuration/#media-paths) explains how the two settings combine. If you open Outtake from another machine, set `OUTTAKE_PUBLIC_BASE_URL` to the URL you use, as [Opening Outtake from another machine](/setup/configuration/#opening-outtake-from-another-machine) describes.

### Docker without Compose

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

The image runs `/outtake server start`.

### Build the image from source

From a checkout of the repository, copy [`.env.example`](https://github.com/PapagoLabs/outtake/blob/main/.env.example) to `.env`, set `OUTTAKE_MEDIA_PATH` in it, and build:

```bash
cp .env.example .env
docker compose up --build
```

That builds `build/docker/Dockerfile.dev`, which bundles the same FFmpeg as the published image.

### A host binary

Tagged [releases](https://github.com/PapagoLabs/outtake/releases) are not published yet, so use the image for now. A host binary needs `ffmpeg` and `ffprobe` from FFmpeg 8 or later, built with libx264, libx265, and zimg, on `PATH` or named by `OUTTAKE_FFMPEG_PATH` and `OUTTAKE_FFPROBE_PATH`. Earlier versions copy HDR metadata into SDR clips and write HEVC files some players refuse. It listens on `0.0.0.0:8080` and keeps its database and clips under `~/.local/share/outtake/` on Linux.

```bash
./outtake server start
```

For a cluster, see [Kubernetes](/setup/kubernetes/).

## Next steps

Open Outtake and [log in with Plex](/guide/logging-in/). The first account to log in owns it.
