# --- Frontend build (output is platform-independent JS; build natively
# on the host arch instead of emulating the target arch under QEMU) ---
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Go build (also run natively; Go cross-compiles without needing
# QEMU to emulate the compiler itself) ---
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=docker
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /seedstrem ./cmd/seedstrem

# --- Runtime (LinuxServer.io base: s6-overlay + PUID/PGID user mapping so
# bind-mounted /config and /data adopt the host user's ownership, matching
# the rest of a Saltbox/LinuxServer stack. The app runs unprivileged as the
# mapped "abc" user, not root.) ---
FROM lscr.io/linuxserver/baseimage-ubuntu:jammy

ARG DEBIAN_FRONTEND="noninteractive"
ARG PUID=1000
ARG PGID=1000
ENV PUID=${PUID}
ENV PGID=${PGID}

# Persist the SQLite DB on the writable /config volume. The app default is a
# relative path (seedstrem.db), which would land in the s6 service CWD (/),
# unwritable by the mapped user. An absolute path under /config keeps it on a
# persisted volume owned by PUID/PGID.
ENV SEEDSTREM_STORAGE_DATABASE=/config/seedstrem.db

# ca-certificates for outbound HTTPS (Prowlarr/indexers); mime-support so
# streamed media gets correct Content-Type headers for Stremio playback.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates mime-support && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /seedstrem /seedstrem

# s6-overlay service definition (runs seedstrem as the mapped user).
COPY docker/root/ /
RUN chmod +x /etc/s6-overlay/s6-rc.d/svc-seedstrem/run

VOLUME ["/config", "/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD ["/seedstrem", "--healthcheck"]

# NOTE: no ENTRYPOINT/CMD — the LinuxServer base image's /init (s6-overlay)
# is the entrypoint. It runs init-adduser (PUID/PGID mapping) then starts the
# svc-seedstrem service defined under docker/root/.
