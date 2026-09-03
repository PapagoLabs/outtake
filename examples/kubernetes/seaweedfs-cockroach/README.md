# Suggested manifests: SeaweedFS + CockroachDB

Development-only single-node Cockroach (`--insecure`, `sslmode=disable`). A NetworkPolicy limits SQL to the Outtake pod. Do not use this as a production topology. Set the NFS server/path to Plex media (clips stay on S3). Replace the `outtake-s3` keys (and matching `config.json` identity) before apply.

```sh
kubectl apply -k examples/kubernetes/seaweedfs-cockroach
```
