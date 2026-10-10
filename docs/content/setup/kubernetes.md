---
title: Kubernetes
description: Run Outtake in a cluster with the example manifests or the example Helm chart
weight: 4
---

There is no published Helm repository or OCI chart. The examples live in the repository under [`examples/kubernetes/`](https://github.com/PapagoLabs/outtake/tree/main/examples/kubernetes), and use the image `ghcr.io/papagolabs/outtake:latest`.

In a cluster, Outtake usually reads the Plex media from an NFS share and keeps its clips in S3 and its data in Postgres. The media share is only ever read: clips and the database never live on it.

## Example manifests

Two Kustomize stacks run Outtake with SeaweedFS for S3 and a database:

- [`seaweedfs-cnpg`](https://github.com/PapagoLabs/outtake/tree/main/examples/kubernetes/seaweedfs-cnpg) uses CloudNativePG for Postgres. Install the CloudNativePG operator first.
- [`seaweedfs-cockroach`](https://github.com/PapagoLabs/outtake/tree/main/examples/kubernetes/seaweedfs-cockroach) uses a single-node CockroachDB started with `--insecure`. It is for development only, so do not use that topology in production.

Before applying either one:

1. Replace `nfs.example.internal` and `/export/plex` with your Plex media NFS server and export.
2. Replace the `outtake-s3` keys, and the matching identity in SeaweedFS's `config.json`.

Then apply it:

```bash
kubectl apply -k examples/kubernetes/seaweedfs-cnpg
```

or

```bash
kubectl apply -k examples/kubernetes/seaweedfs-cockroach
```

## Example Helm chart

[`examples/kubernetes/helm/outtake`](https://github.com/PapagoLabs/outtake/tree/main/examples/kubernetes/helm/outtake) is an example chart to install from a checkout. Do not `helm repo add` it. Its [README](https://github.com/PapagoLabs/outtake/blob/main/examples/kubernetes/helm/outtake/README.md) lists the values.

- The one value every install needs is the media: set `media.nfs.server` and `media.nfs.path`, or `media.existingClaim` for a claim you made.
- Set `media.plexMediaRoot` when Plex reports a different prefix for the files, as [Media paths](/setup/configuration/#media-paths) describes.
- Without anything else enabled, the chart uses the same defaults as Compose: files on a volume and SQLite.
- Enable at most one file backend, `backends.seaweedfs` or `backends.rustfs`, and at most one database backend, `backends.cockroach` or `backends.cnpg`.

## Configuration

Every setting is an `OUTTAKE_*` environment variable on the container (see [Configuration](/setup/configuration/)). Set `OUTTAKE_PUBLIC_BASE_URL` to the URL of your ingress, so Plex login returns there and Outtake answers to that host name.
