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
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /seedstrem ./cmd/seedstrem \
    && mkdir /empty

# --- Runtime ---
FROM gcr.io/distroless/static:nonroot
COPY --from=build /seedstrem /seedstrem
# Writable config dir for the nonroot user (uid 65532).
COPY --from=build --chown=nonroot:nonroot /empty /config
COPY --from=build --chown=nonroot:nonroot /empty /data
VOLUME ["/config", "/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD ["/seedstrem", "--healthcheck"]
ENTRYPOINT ["/seedstrem"]
