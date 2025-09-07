# --- UI build ---
FROM node:20-alpine AS ui
WORKDIR /ui
COPY ui/package.json ./
RUN npm install --no-audit --no-fund
COPY ui/ .
RUN npm run build

# --- Go build ---
FROM golang:1.23-alpine AS api
WORKDIR /src/api
COPY src/api/go.mod ./
RUN go mod download
COPY src/api/ .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/ddui

# --- Runtime ---
FROM debian:bookworm-slim

LABEL maintainer="SoFMeRight <sofmeright@gmail.com>" \
      org.opencontainers.image.title="DDUI (Designated Driver UI)" \
      description="Sometimes you need someone else to take the wheel... DDUI is a declarative, security-first Docker orchestration engine. Please Docker responsibly." \
      org.opencontainers.image.description="Sometimes you need someone else to take the wheel... DDUI is a declarative, security-first Docker orchestration engine. Please Docker responsibly." \
      org.opencontainers.image.source="https://github.com/sofmeright/DDUI.git" \
      org.opencontainers.image.licenses="GPL-3.0"

# Base deps (curl for healthcheck + downloads; ssh for DOCKER_HOST=ssh://; tzdata; ca-certs)
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
    curl -fsSL -o /usr/local/bin/sops https://github.com/getsops/sops/releases/download/v3.10.2/sops-v3.10.2.linux.amd64; \
    chmod +x /usr/local/bin/sops

WORKDIR /home/ddui
COPY --from=api /bin/ddui /usr/local/bin/ddui
COPY --from=ui /ui/dist ./ui/dist

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
      CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1

CMD ["ddui"]