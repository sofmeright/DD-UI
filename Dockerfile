# --- UI build ---
FROM node:20-alpine AS ui
WORKDIR /ui
# copy package manifest(s) — lockfile is optional
COPY ui/package.json ./
COPY ui/package-lock.json* ./
# if lockfile exists -> ci, else -> install
RUN [ -f package-lock.json ] && npm ci --no-audit --no-fund || npm install --no-audit --no-fund
COPY ui/ .
RUN npm run build
    
# --- Go build ---
FROM golang:1.22-alpine AS api
WORKDIR /src/api
RUN apk add --no-cache git ca-certificates

# prime the cache with go.mod, then fetch
COPY src/api/go.mod ./
RUN go mod download

# now copy sources, then tidy to generate go.sum
COPY src/api/ .
RUN go mod tidy

# build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /bin/ddui .
    
# --- Runtime ---
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /home/ddui

# binary + UI assets
COPY --from=api /bin/ddui /usr/local/bin/ddui
COPY --from=ui /ui/dist ./ui/dist

ENV RUST_LOG=info
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1

CMD ["ddui"]