---
title: Storage and Databases
description: Where Outtake keeps clips, previews, and its database, and how to use S3 or Postgres instead
weight: 3
---

Outtake keeps two kinds of data: the files it renders, and a database of clips, profiles, settings, and sessions. Neither lives with your media. The media mount is read-only, and Outtake never writes to it.

## Files

By default files go on the local filesystem under `OUTTAKE_STORAGE_PATH`, which is `/data/output` in the Docker images:

| Directory | Holds |
| --- | --- |
| `clips/` | Video clips, and the SDR versions of clips that keep HDR |
| `gifs/` | GIFs |
| `screenshots/` | Screenshots |
| `previews/` | Previews from the export form. Outtake keeps the recent ones and deletes the rest |

Every render writes to a temporary file beside its destination and moves it into place only when FFmpeg succeeds, so a failed or canceled render, including a regenerate, never leaves a broken file behind. Temporary files a crash left behind are swept on the next start.

### S3

To keep files in an S3-compatible store instead, such as SeaweedFS, RustFS, MinIO, or AWS S3, set:

```bash
OUTTAKE_STORAGE_BACKEND=s3
OUTTAKE_S3_ENDPOINT=https://s3.example.com
OUTTAKE_S3_BUCKET=outtake
OUTTAKE_S3_REGION=us-east-1
OUTTAKE_S3_ACCESS_KEY=...
OUTTAKE_S3_SECRET_KEY=...
OUTTAKE_S3_USE_PATH_STYLE=true
```

FFmpeg needs local files, so `OUTTAKE_STORAGE_PATH` is still used as a scratch directory: renders are written there and uploaded, and a file is downloaded there again when you play or download it and it is not present locally. Path-style URLs are on by default, which SeaweedFS and RustFS expect. Turn them off for a store that wants virtual-hosted URLs.

## Database

### SQLite

The default is a SQLite database at `OUTTAKE_DATABASE_PATH`, which is `/data/outtake.db` in the Docker images. It needs no setup. Keep the whole `/data` volume together, because SQLite also writes `outtake.db-wal` and `outtake.db-shm` beside the database.

### Postgres

For Postgres, set:

```bash
OUTTAKE_DATABASE_BACKEND=postgres
OUTTAKE_DATABASE_URL=postgres://outtake:secret@db.example.com:5432/outtake?sslmode=require
```

`OUTTAKE_DATABASE_URL` is a standard connection string. Outtake creates and upgrades its tables itself when it starts, on either database, so there is nothing to run by hand.

## Backups

With the defaults, everything Outtake keeps is in the `/data` volume: back it up as a whole, while Outtake is stopped. With S3 or Postgres, back up the bucket and the database the way you back up anything else on them.
