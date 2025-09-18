# --- UI build ---
FROM node:20-alpine AS ui
WORKDIR /ui

# copy lockfile if it exists
COPY ui/package.json ui/package-lock.json* ./

# use npm ci if lockfile is present; otherwise, npm install
RUN sh -c 'if [ -f package-lock.json ]; then \
  npm ci --no-audit --no-fund; \
else \
  echo "No package-lock.json found; running npm install"; \
  npm install --no-audit --no-fund; \
fi'

COPY ui/ .
RUN npm run build

# --- Go build ---
FROM golang:1.25.1-alpine AS api
WORKDIR /api
COPY api/go.mod ./
RUN go mod download
COPY api/ .
ARG TARGETOS
ARG TARGETARCH
RUN go mod tidy
# fallback to linux/amd64 if buildx args aren't provided
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -o /bin/dd-ui .

# --- Runtime ---
# Using Alpine for smaller size and better security maintenance
FROM alpine:3.21.0

LABEL maintainer="SoFMeRight <sofmeright@gmail.com>" \
      org.opencontainers.image.title="DD-UI (Designated Driver UI)" \
      description="Sometimes you need someone else to take the wheel... DD-UI is a declarative, security-first Docker orchestration engine. Please Docker responsibly." \
      org.opencontainers.image.description="Sometimes you need someone else to take the wheel... DD-UI is a declarative, security-first Docker orchestration engine. Please Docker responsibly." \
      org.opencontainers.image.source="https://github.com/sofmeright/DD-UI.git" \
      org.opencontainers.image.licenses="GPL-3.0"

# Base deps (curl for healthcheck + downloads; ssh for DOCKER_HOST=ssh://; tzdata; ca-certs)
# Alpine uses musl instead of glibc, avoiding the zlib1g vulnerability
RUN apk update && \
      apk upgrade && \
      apk add --no-cache \
      ca-certificates curl openssh-client tzdata bash && \
      rm -rf /var/cache/apk/*

# --- Docker CLI (static) ---
ARG DOCKER_CLI_VERSION=28.4.0
RUN set -eux; \
      # Alpine uses different arch detection
      arch="$(uname -m)"; \
      case "$arch" in \
      x86_64)  dl_arch="x86_64" ;; \
      aarch64) dl_arch="aarch64" ;; \
      armv7l)  dl_arch="armhf" ;; \
      *) echo "unsupported arch: $arch" >&2; exit 1 ;; \
      esac; \
      curl -fsSL "https://download.docker.com/linux/static/stable/${dl_arch}/docker-${DOCKER_CLI_VERSION}.tgz" -o /tmp/docker.tgz; \
      tar -xzf /tmp/docker.tgz -C /usr/local/bin --strip-components=1 docker/docker; \
      rm -f /tmp/docker.tgz; \
      docker --version

# --- Compose v2 plugin ---
ARG COMPOSE_VERSION=2.39.3
RUN set -eux; \
      # Alpine uses different arch detection
      arch="$(uname -m)"; \
      case "$arch" in \
      x86_64)  comp_arch="x86_64" ;; \
      aarch64) comp_arch="aarch64" ;; \
      armv7l)  comp_arch="armv7" ;; \
      *) echo "unsupported arch: $arch" >&2; exit 1 ;; \
      esac; \
      mkdir -p /usr/local/lib/docker/cli-plugins; \
      curl -fsSL -o /usr/local/lib/docker/cli-plugins/docker-compose \
      "https://github.com/docker/compose/releases/download/v${COMPOSE_VERSION}/docker-compose-linux-${comp_arch}"; \
      chmod +x /usr/local/lib/docker/cli-plugins/docker-compose; \
      docker compose version

# Build SOPS from source with Go 1.25.1 (latest stable) to avoid all vulnerabilities
# The pre-built binaries and Alpine's Go have vulnerabilities
ARG SOPS_VERSION=3.10.2
ARG GO_VERSION=1.25.1
RUN set -eux; \
    # Install build dependencies \
    apk add --no-cache --virtual .build-deps git && \
    # Download and install Go 1.25.1 to build SOPS without vulnerabilities \
    arch="$(uname -m)"; \
    case "$arch" in \
    x86_64)  go_arch="amd64" ;; \
    aarch64) go_arch="arm64" ;; \
    armv7l)  go_arch="armv6l" ;; \
    *) echo "unsupported arch: $arch" >&2; exit 1 ;; \
    esac; \
    wget -O /tmp/go.tar.gz "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz" && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz && \
    export PATH="/usr/local/go/bin:$PATH" && \
    # Build SOPS from source \
    git clone --depth 1 --branch v${SOPS_VERSION} https://github.com/getsops/sops.git /tmp/sops && \
    cd /tmp/sops && \
    # Force update cloudflare/circl to v1.6.1 to fix CVE-2025-8556 \
    /usr/local/go/bin/go get github.com/cloudflare/circl@v1.6.1 && \
    /usr/local/go/bin/go mod tidy && \
    CGO_ENABLED=0 /usr/local/go/bin/go build -o /usr/local/bin/sops ./cmd/sops && \
    # Cleanup \
    cd / && \
    rm -rf /tmp/sops /usr/local/go /root/go && \
    apk del .build-deps && \
    chmod +x /usr/local/bin/sops

RUN docker --version && docker compose version && sops --version

WORKDIR /app
COPY --from=api /bin/dd-ui /usr/local/bin/dd-ui
COPY --from=ui /ui/dist ./ui/dist
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh

# (optional, rootless mode?)
# RUN useradd -r -u 10001 -g root dd-ui
# USER dd-ui

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD curl -fsSk https://127.0.0.1:443/healthz || exit 1

ENTRYPOINT ["/entrypoint.sh"]
CMD ["dd-ui"]