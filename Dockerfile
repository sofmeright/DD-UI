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
COPY src/*.go .     # bring in top-level main files (db.go, db_hosts.go, etc.)
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/ddui

# --- Runtime ---
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /home/ddui
COPY --from=api /bin/ddui /usr/local/bin/ddui
COPY --from=ui /ui/dist ./ui/dist
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1
ENV RUST_LOG=info
CMD ["ddui"]