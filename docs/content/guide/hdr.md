---
title: HDR and Playback
description: When a clip keeps HDR, what plays in your browser, and which sources Outtake refuses
weight: 4
---

## Keeping HDR

Whether a video clip keeps HDR is a setting of its [profile](/guide/profiles/), **HDR**. It matters only for a video clip from an HDR source, either HDR10 (PQ) or HLG.

- **HDR on** keeps the source's HDR. The clip is a 10-bit HEVC (H.265) file that carries the source's mastering display and light level data unchanged. Phones, Apple devices, and YouTube take it as HDR.
- **HDR off** tone-maps the clip to SDR: an 8-bit H.264 file in standard BT.709 color, which upload sites and editors expect, and which most social sites need.

The built-in **4K HDR** profile keeps HDR, and **720p**, **1080p**, and **4K** do not. GIFs and screenshots are always SDR, because neither format can carry HDR, so an HDR source is tone-mapped for them too.

A clip takes the setting each time it renders. To change a finished clip, change its profile or pick another, then choose **Regenerate**.

HEVC encodes are several times slower than H.264, so a clip that keeps HDR is allowed 60 times its own length to render, where any other is allowed 20 times, never less than 30 minutes. `OUTTAKE_FFMPEG_TIMEOUT_SEC` sets a fixed limit instead.

## What your browser plays

Many screens and browsers cannot show HDR, so Outtake renders an SDR version beside every clip that keeps HDR from an HDR source. It renders after the clip itself, and the clip's card shows a second progress bar for it. The clip stays processing until both are done. If the SDR version fails, the clip still completes.

- A clip's card plays the HDR file when your screen shows HDR and your browser plays HEVC Main 10, and the SDR version everywhere else.
- The preview on the export form follows the same rule.
- Each player's badge names what it plays, such as **HDR · 4K** or **SDR · 1080p**.
- **Download** always gives you the clip itself, with HDR. There is no SDR download.

The SDR versions and every other preview are capped by the **Maximum Preview Resolution** under **Settings → Previews**.

### Browser notes

- Brave and Chrome on Linux decode HEVC only with hardware video decoding. Without it they play the SDR version, which works everywhere.
- Where Chromium does decode HEVC, versions 151 and later draw 10-bit video black on NVIDIA under Wayland. Starting the browser with `--ozone-platform=x11` avoids that.
- An HDR clip rendered before Outtake made SDR versions has none, and its card says so on a screen that cannot show it. Regenerate the clip to add one.

## Dolby Vision

Outtake refuses a Dolby Vision source with no displayable base layer, such as Dolby Vision profile 5, with "This Dolby Vision file can't be exported with correct colors". FFmpeg decodes only the base layer, which in profile 5 is in Dolby's own color space, so the result would have the wrong colors. Use a copy of the title with an HDR10 base layer.
