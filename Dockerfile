# remuda — browser/phone client for herdr.
#
# Stages:
#   web      builds the React SPA
#   build    compiles the Go binary with the SPA embedded
#   test     runs go vet + go test + the TypeScript typecheck (docker build --target test)
#   runtime  the image to run: the binary, herdr (for `terminal attach`), ssh, git
#
# The container is only the bridge: herdr itself keeps running on the host.
# See compose.yaml for the mounts it needs (herdr socket, ~/.claude, ~/.ssh,
# and your projects at the same paths so the file viewer can read them).

ARG GO_VERSION=1.27
ARG NODE_VERSION=22

# ---- web ----
FROM node:${NODE_VERSION}-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- build ----
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/remuda ./cmd/remuda

# ---- test ----
FROM build AS test
RUN apk add --no-cache nodejs npm
COPY --from=web /src/web/node_modules ./web/node_modules
RUN go vet ./... && go test ./... && cd web && npm run typecheck

# ---- runtime ----
FROM debian:bookworm-slim AS runtime
ARG HERDR_INSTALL=0
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl openssh-client git \
 && rm -rf /var/lib/apt/lists/*
# herdr's own installer: detects linux/{x86_64,aarch64} and verifies the checksum.
# HERDR_INSTALL=1 runs herdr's installer at build time — an unpinned
# `curl | sh`, so it is off by default: mount the host's binary, or set it
# knowing that.
RUN if [ "$HERDR_INSTALL" = "1" ]; then \
      HERDR_INSTALL_DIR=/usr/local/bin sh -c "curl -fsSL https://herdr.dev/install.sh | sh"; \
    fi
COPY --from=build /out/remuda /usr/local/bin/remuda

# Runs as the uid you pass (compose: `user: "${UID}:${GID}"`) so files under
# the mounted home keep your ownership; HOME must match the mount point.
# Without one it still is not root: an unprivileged uid, and the home open
# to it.
ENV HOME=/home/herdr
RUN mkdir -p /home/herdr && chmod 777 /home/herdr
WORKDIR /home/herdr
USER 1000:1000
EXPOSE 7317
# Loopback-only is the binary's default; inside a container the port mapping
# is the boundary, so listen on all interfaces here.
ENTRYPOINT ["remuda"]
CMD ["-addr", "0.0.0.0:7317"]
