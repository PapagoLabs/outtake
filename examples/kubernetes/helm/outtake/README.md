# Outtake Helm chart

Example chart under `examples/kubernetes/helm/outtake`. PapagoLabs has no Helm repo; install from this tree after a checkout, not via `helm repo add`.

`docker-compose.yml` remains the local FS + SQLite path. Suggested plain manifests live in `examples/kubernetes/seaweedfs-cnpg/` and `examples/kubernetes/seaweedfs-cockroach/`.

The site input is Plex media: set `media.nfs.server` and `media.nfs.path`, or `media.existingClaim`. Clips stay on `storage-path` or S3.

## Values (App config knobs)

Env is `OUTTAKE_*`. Defaults stay local.

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
| `media.nfs.server` / `path` or `media.existingClaim` | — | required site input for Plex media |

## Optional backends

From a repo checkout:

```
helm dependency update examples/kubernetes/helm/outtake
helm install outtake examples/kubernetes/helm/outtake
```

Helm checks Chart.yaml deps even when backends are disabled. Enable at most one blob and one DB:

| Value | Stands up |
| --- | --- |
| `backends.seaweedfs.enabled` | SeaweedFS S3 (`4.45.0`) |
| `backends.rustfs.enabled` | RustFS S3 (`1.0.0-rc.5`) |
| `backends.cockroach.enabled` | CockroachDB (`22.0.3`) |
| `backends.cnpg.enabled` | CloudNativePG cluster (`0.8.1`) |
| `backends.cnpgOperator.enabled` | CloudNativePG operator (`0.29.0`) |

See `examples/values-distributed.yaml` (SeaweedFS + Cockroach; NFS is the remaining site input).
