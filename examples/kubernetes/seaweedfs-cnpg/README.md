# Suggested manifests: SeaweedFS + CloudNativePG

Install the CloudNativePG operator first. Set `nfs.example.internal` / `/export/plex` to the Plex media NFS (clips stay on S3). Replace the `outtake-s3` keys (and matching `config.json` identity) before apply.

```sh
kubectl apply -k examples/kubernetes/seaweedfs-cnpg
```

`OUTTAKE_DATABASE_URL` is the CNPG app secret `outtake-pg-app` (`uri`).
