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
FROM golang:1.24-alpine AS api
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
FROM debian:bookworm-slim

LABEL maintainer="SoFMeRight <sofmeright@gmail.com>" \
      org.opencontainers.image.title="DD-UI (Designated Driver UI)" \
      description="Sometimes you need someone else to take the wheel... DD-UI is a declarative, security-first Docker orchestration engine. Please Docker responsibly." \
      org.opencontainers.image.description="Sometimes you need someone else to take the wheel... DD-UI is a declarative, security-first Docker orchestration engine. Please Docker responsibly." \
      org.opencontainers.image.source="https://github.com/sofmeright/DD-UI.git" \
      org.opencontainers.image.licenses="GPL-3.0"

# Base deps (curl for healthcheck + downloads; ssh for DOCKER_HOST=ssh://; tzdata; ca-certs)
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl openssh-client tzdata \
      && rm -rf /var/lib/apt/lists/*

# --- Docker CLI (static) ---
ARG DOCKER_CLI_VERSION=26.1.4
RUN set -eux; \
      deb_arch="$(dpkg --print-architecture)"; \
      case "$deb_arch" in \
      amd64)  dl_arch="x86_64" ;; \
      arm64)  dl_arch="aarch64" ;; \
      armhf)  dl_arch="armhf" ;; \
      *) echo "unsupported arch: $deb_arch" >&2; exit 1 ;; \
      esac; \
      curl -fsSL "https://download.docker.com/linux/static/stable/${dl_arch}/docker-${DOCKER_CLI_VERSION}.tgz" -o /tmp/docker.tgz; \
      tar -xzf /tmp/docker.tgz -C /usr/local/bin --strip-components=1 docker/docker; \
      rm -f /tmp/docker.tgz; \
      docker --version

# --- Compose v2 plugin ---
ARG COMPOSE_VERSION=2.28.1
RUN set -eux; \
      deb_arch="$(dpkg --print-architecture)"; \
      case "$deb_arch" in \
      amd64)  comp_arch="x86_64" ;; \
      arm64)  comp_arch="aarch64" ;; \
      armhf)  comp_arch="armv7" ;; \
      *) echo "unsupported arch: $deb_arch" >&2; exit 1 ;; \
      esac; \
      mkdir -p /usr/local/lib/docker/cli-plugins; \
      curl -fsSL -o /usr/local/lib/docker/cli-plugins/docker-compose \
      "https://github.com/docker/compose/releases/download/v${COMPOSE_VERSION}/docker-compose-linux-${comp_arch}"; \
      chmod +x /usr/local/lib/docker/cli-plugins/docker-compose; \
      docker compose version

# Install SOPS (v3.10.2)
RUN set -eux; \
    curl -fsSL -o /usr/local/bin/sops \
      https://github.com/getsops/sops/releases/download/v3.10.2/sops-v3.10.2.linux.amd64; \
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