# Copyright 2026 Nicholas Fedor <nick@nickfedor.com>
# SPDX-License-Identifier: AGPL-3.0-or-later
#
# Production Dockerfile for outtake.
# Expects a GoReleaser dockers_v2 context with the pre-built binary at
# ${TARGETPLATFORM}/. ffmpeg and ffprobe are static binaries copied from a
# pinned image for the target platform.

ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG BASE_IMAGE=alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
ARG FFMPEG_IMAGE=mwader/static-ffmpeg:9.0.2@sha256:7d9bdaaf887f7e6ce6151f67325c344074b5ff1fb75316011c3376503e449a7b

# Pulled for the target platform, so an arm64 image gets arm64 binaries.
FROM ${FFMPEG_IMAGE} AS ffmpeg

FROM --platform=$BUILDPLATFORM ${BASE_IMAGE} AS builder

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 1000 -g 1000 appuser \
    && mkdir -p /data/output

FROM scratch

ARG TARGETPLATFORM

LABEL \
    org.opencontainers.image.title="outtake" \
    org.opencontainers.image.description="A clip manager for Plex libraries." \
    org.opencontainers.image.source="https://github.com/PapagoLabs/outtake" \
    org.opencontainers.image.licenses="AGPL-3.0-or-later"

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

COPY --from=ffmpeg --chmod=0755 /ffmpeg /usr/bin/ffmpeg
COPY --from=ffmpeg --chmod=0755 /ffprobe /usr/bin/ffprobe
COPY --chmod=0755 ${TARGETPLATFORM}/outtake /outtake

EXPOSE 8080

# A new named volume takes the ownership of the image's /data, so the server
# can create its database and output directory there as uid 1000.
COPY --from=builder --chown=1000:1000 /data /data

VOLUME ["/data"]

ENV OUTTAKE_DATABASE_PATH=/data/outtake.db
ENV OUTTAKE_STORAGE_PATH=/data/output
ENV OUTTAKE_LISTEN_ADDR=:8080
ENV OUTTAKE_FFMPEG_PATH=/usr/bin/ffmpeg
ENV OUTTAKE_FFPROBE_PATH=/usr/bin/ffprobe
ENV OUTTAKE_PUBLIC_BASE_URL=http://localhost:8080

USER appuser:appuser

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/outtake", "health"]

ENTRYPOINT ["/outtake"]
CMD ["server", "start"]
