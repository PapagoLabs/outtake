# Copyright 2026 Nicholas Fedor <nick@nickfedor.com>
# SPDX-License-Identifier: AGPL-3.0-or-later
#
# Production Dockerfile for outtake.
# Expects a GoReleaser dockers_v2 context with the pre-built binary and
# static ffmpeg/ffprobe at ${TARGETPLATFORM}/.

ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG BASE_IMAGE=alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

FROM --platform=$BUILDPLATFORM ${BASE_IMAGE} AS builder

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 1000 -g 1000 appuser

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

COPY --chmod=0755 ${TARGETPLATFORM}/ffmpeg /usr/bin/ffmpeg
COPY --chmod=0755 ${TARGETPLATFORM}/ffprobe /usr/bin/ffprobe
COPY --chmod=0755 ${TARGETPLATFORM}/outtake /outtake

EXPOSE 8080

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
