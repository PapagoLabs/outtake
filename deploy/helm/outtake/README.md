# Outtake Helm chart

Packages Outtake for Kubernetes. `docker-compose.yml` remains the local FS + SQLite path.

## Values (App config knobs)

Env is `OUTTAKE_*`. Defaults stay local:

| Value | Env | Default |
| --- | --- | --- |
| `outtake.storageBackend` | `OUTTAKE_STORAGE_BACKEND` | `filesystem` |
| `outtake.storagePath` | `OUTTAKE_STORAGE_PATH` | `/data/output` |
| `outtake.s3.*` | `OUTTAKE_S3_*` | path-style S3, region `us-east-1` |
| `outtake.databaseBackend` | `OUTTAKE_DATABASE_BACKEND` | `sqlite` |
| `outtake.databasePath` | `OUTTAKE_DATABASE_PATH` | `/data/outtake.db` |
| `outtake.databaseUrl` | `OUTTAKE_DATABASE_URL` | pgx DSN |
| `media.localMediaRoot` | `OUTTAKE_LOCAL_MEDIA_ROOT` | `/media` |
| `media.plexMediaRoot` | `OUTTAKE_PLEX_MEDIA_ROOT` | empty |
| `media.nfs.server` / `path` | — | required site input for Plex media |

Blob store is generic S3 (SeaweedFS or RustFS). Database is generic Postgres-protocol (CockroachDB or CNPG). Point `s3.endpoint` and `databaseUrl` at whichever you run.

See `examples/values-distributed.yaml`.
