---
title: Upgrading
description: What changes when you update Outtake, and what to do before and after
weight: 5
---

Outtake upgrades its database on the first start of a new version, so updating is usually a matter of pulling the new image and starting it again. Some versions change how Outtake behaves in ways you need to act on. This page lists them by version.

Before any update, [back up](/setup/storage/#backups) the data volume, or the Postgres database if you use one. Outtake does not go back to an older version's database.

To stay on one version line, pin the image to it, such as `ghcr.io/papagolabs/outtake:0.4`, rather than `latest`.

## From 0.3 to 0.4

Version 0.4 makes Outtake a multi-user application with one owner, changes the built-in clip profiles, and makes Keep HDR a profile setting. Read the whole section before you update.

### Logging in

Every browser now logs in with Plex. In 0.3, a browser that opened Outtake acted as the account that logged in last, so a script or a second browser worked without logging in. Now each needs its own login.

The first login after the update claims Outtake for its account, which becomes the owner:

- When a Plex login was saved by 0.3, only the account that logged in most recently can claim Outtake.
- When a Plex server is chosen, only an account that owns that server can claim Outtake. If the chosen server is one a friend shared with you, the first login is turned away with "This Plex account can't use this Outtake". Run `outtake owner reset`, log in, and choose a server your account owns.

The clips made before the update become the owner's when Outtake is claimed. See [Logging In and Servers](/guide/logging-in/) for the owner and the reset.

Other people you share your Plex server with can now use Outtake too, after the owner or an admin approves them. Members have render limits. See [Users and Limits](/guide/users/).

### Reverse proxies and host names

Outtake now answers only requests addressed to a host it knows, and any other host gets `421 Misdirected Request`. It knows IP addresses, `localhost`, names without dots, private names such as `nas.local`, the host in `OUTTAKE_PUBLIC_BASE_URL`, and the hosts in `OUTTAKE_ALLOWED_HOSTS`.

If you open Outtake through a reverse proxy, an ingress, or a public name, set `OUTTAKE_PUBLIC_BASE_URL` to that address, such as `https://outtake.example.com`, and add any other name you use to `OUTTAKE_ALLOWED_HOSTS`. Names under a public suffix, such as Tailscale's `*.ts.net`, also need to be listed. See [Opening Outtake from another machine](/setup/configuration/#opening-outtake-from-another-machine).

### Clip profiles

The built-in profiles Low, Medium, and High are replaced by profiles named after what they produce:

| 0.3 profile | Clips move to | Its settings |
| --- | --- | --- |
| Low | 720p | CRF 21, medium, 160 kbps |
| Medium | 1080p | CRF 20, medium, 192 kbps |
| High | 4K | CRF 18, slow, 256 kbps |

0.4 also adds **4K HDR**, which keeps HDR, and **Original**, which copies the source's video when it can. **1080p** becomes the default, unless a profile you made yourself was the default, which stays the default.

Profiles you made yourself are kept as they are. A built-in profile you edited or renamed in 0.3 is replaced with the new settings, so note any changes you made to one before you update and make them again as a profile of your own.

### HDR

Keep HDR is now a setting of each clip profile, shown as **HDR** in the profile editor. The **Web-safe color** checkbox and the per-clip HDR choice are gone. Of the built-in profiles, only **4K HDR** and **Original** keep HDR.

A clip from 0.3 keeps the file it has. When you regenerate it, it takes its profile's setting, so a clip that kept HDR and moved to 1080p or 4K comes out in SDR. Choose **4K HDR** or **Original** for it before you regenerate it to keep HDR. See [HDR and Playback](/guide/hdr/).

The settings `OUTTAKE_WEB_SAFE_COLOR` and `OUTTAKE_PRESERVE_HDR` are removed. Outtake logs a warning while either is still set, and otherwise ignores it. Remove them from your configuration.

### The JSON API

If you call the API from scripts:

- Every request needs a logged-in session. There is no request that acts as the owner without one.
- A request that sends `preserveHdr` or `webSafeColor` is refused with `400`. Choose a profile that keeps HDR instead.
- The profile ids `low`, `medium`, and `high` are gone, and a request naming one is refused with `invalid_quality`. Profile ids are generated, so read them from **Settings → Clip Profiles**, or leave `quality` out to use the default profile.
- A clip's `error` holds a plain message, and `errorDetails` holds the technical report. A clip no longer reports `webSafeColor`, and reports `shared`.
- A failed preview's status reports `message` and `details`.
- Error responses add a `details` field beside `error` and `message`.

### The image

The image now carries FFmpeg 9 and its data directory is owned by the user it runs as, so a fresh volume works without a manual `chown`. A host binary needs FFmpeg 8 or later. See [Install](/setup/install/).
