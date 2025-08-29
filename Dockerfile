# --- UI build ---
    FROM node:20-alpine AS ui
    WORKDIR /ui
    COPY ui/package*.json ./
    RUN npm ci --include=dev --no-audit --no-fund || npm i --include=dev --no-audit --no-fund
    COPY ui/ .
    RUN npm run build
    
    # --- Go build ---
    FROM golang:1.22-alpine AS api
    WORKDIR /src/api
    COPY src/api/go.mod src/api/go.sum ./
    RUN go mod download
    COPY src/api/ .
    RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/ddui
    
    # --- Runtime ---
    FROM debian:bookworm-slim
    RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata && rm -rf /var/lib/apt/lists/*
    WORKDIR /home/ddui
    COPY --from=api /bin/ddui /usr/local/bin/ddui
    COPY --from=ui /ui/dist /opt/ddui/ui
    ENV UI_DIST=/opt/ddui/ui
    ENV BIND_ADDR=:8080
    EXPOSE 8080
    HEALTHCHECK --interval=15s --timeout=2s --retries=3 CMD curl -fsS http://127.0.0.1:8080/api/healthz >/dev/null || exit 1
    USER 65532:65532
    ENTRYPOINT ["/usr/local/bin/ddui"]