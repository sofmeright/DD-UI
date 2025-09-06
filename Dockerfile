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

# minimal deps (you already had curl/ca-certs/ssh/tzdata)
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl openssh-client tzdata \
      && rm -rf /var/lib/apt/lists/*

# --- add Docker CLI (static) ---
ARG DOCKER_CLI_VERSION=26.1.4
RUN set -eux; \
      arch="$(dpkg --print-architecture)"; \
      curl -fsSL "https://download.docker.com/linux/static/stable/${arch}/docker-${DOCKER_CLI_VERSION}.tgz" -o /tmp/docker.tgz; \
      tar -xzf /tmp/docker.tgz -C /usr/local/bin --strip-components=1 docker/docker; \
      rm /tmp/docker.tgz

# --- add Compose v2 plugin ---
ARG COMPOSE_VERSION=2.28.1
RUN set -eux; \
      mkdir -p /usr/libexec/docker/cli-plugins; \
      curl -fsSL -o /usr/libexec/docker/cli-plugins/docker-compose \
      "https://github.com/docker/compose/releases/download/v${COMPOSE_VERSION}/docker-compose-linux-$(uname -m)"; \
      chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# (optional) make sure plugin dir is discoverable (usually not needed on Debian)
ENV PATH="/usr/libexec/docker/cli-plugins:${PATH}"

WORKDIR /home/ddui
COPY --from=api /bin/ddui /usr/local/bin/ddui
COPY --from=ui /ui/dist ./ui/dist

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1
CMD ["ddui"]