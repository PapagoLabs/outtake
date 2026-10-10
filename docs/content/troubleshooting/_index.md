---
title: Troubleshooting
description: Fixes for login, servers, media paths, storage, HDR playback, and how to report a problem
weight: 3
type: docs
---

## Login

**Login never finishes.** Approve Outtake in the Plex popup. If you open Outtake through a host name other than `localhost`, set `OUTTAKE_PUBLIC_BASE_URL` to that URL, because Plex returns there.

**"This Outtake belongs to a different Plex account."** Log in with the owner's Plex account, or [reset the owner](/guide/logging-in/#handing-outtake-to-another-account).

**421 Misdirected Request.** You reached Outtake through a host name it does not know. Set `OUTTAKE_PUBLIC_BASE_URL` to the URL you use, or add the host to `OUTTAKE_ALLOWED_HOSTS` (see [Opening Outtake from another machine](/setup/configuration/#opening-outtake-from-another-machine)).

## Servers and media

**No Plex servers found.** Enter the server's address under **Custom Server URL** on the servers page. Outtake must be able to reach that address.

**No media found.** Choose a server first, then search or browse again.

**The server refused its token.** Log in again. Each login refreshes the token Outtake uses for the chosen server.

**Clips fail, or files are missing.** Outtake must be able to read the file at the path Plex reports. In Docker, mount the library and set `OUTTAKE_LOCAL_MEDIA_ROOT`, and `OUTTAKE_PLEX_MEDIA_ROOT` when Plex reports a different prefix, so the path lands on the mount (see [Media paths](/setup/configuration/#media-paths)). On Kubernetes that mount is the media NFS share, or `media.nfs` or `media.existingClaim` on the example chart.

## Storage

**Outtake exits at start with `permission denied` on `/data`.** The data volume must be writable by uid 1000, the user the image runs as. A fresh named volume is. A bind mount needs `chown 1000:1000` on the host directory.

**Wrong storage or database.** The defaults are `filesystem` and `sqlite`. For S3, set `OUTTAKE_STORAGE_BACKEND=s3` and the `OUTTAKE_S3_*` variables. For Postgres, set `OUTTAKE_DATABASE_BACKEND=postgres` and `OUTTAKE_DATABASE_URL` (see [Storage and Databases](/setup/storage/)).

**Kubernetes apply fails.** Set the NFS server and path to your Plex library, and replace the `outtake-s3` keys before applying. For `seaweedfs-cnpg`, install the CloudNativePG operator first.

## Playback and HDR

**"This browser can't play this clip".** The browser cannot decode the clip's video. A clip that keeps HDR is HEVC, which Brave and Chrome on Linux decode only with hardware video decoding. Its card normally plays an SDR version instead, so this appears for an HDR clip rendered before Outtake made SDR versions, whose card says to regenerate it. The file itself is fine: download it, open it in another browser, or regenerate the clip.

**HDR video is black in Chromium.** Chromium 151 and later draw 10-bit video black on NVIDIA under Wayland. Start the browser with `--ozone-platform=x11`.

**"This Dolby Vision file can't be exported with correct colors".** The source is Dolby Vision with no displayable base layer, such as profile 5. Use a copy of the title with an HDR10 base layer (see [Dolby Vision](/guide/hdr/#dolby-vision)).

**A render stops with a timeout.** A long clip, especially at 4K with HDR, can take longer than the default limit on a slow host. Set `OUTTAKE_FFMPEG_TIMEOUT_SEC` to a longer limit, in seconds.

## A host binary

**FFmpeg or ffprobe errors.** Install both, from FFmpeg 8 or later, and keep them on `PATH`, or set `OUTTAKE_FFMPEG_PATH` and `OUTTAKE_FFPROBE_PATH`. The Docker images include them.

## Reporting a problem

An error that needs more than its message has a **Details** section under it, on a page, on a failed clip's card, and under a failed preview. Open it and choose **Copy**, or select the text, and paste it into an [issue](https://github.com/PapagoLabs/outtake/issues). Its `ref` matches the line Outtake wrote to its log for the same failure, and Plex tokens are removed from both. Details show only when you are logged in.
