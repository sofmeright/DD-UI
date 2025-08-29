# --- UI build ---
    FROM node:20-alpine AS ui
    WORKDIR /ui
    COPY ui/package.json ui/package-lock.json* ./
    RUN if [ -f package-lock.json ]; then npm ci --include=dev --no-audit --no-fund; else npm i --include=dev --no-audit --no-fund; fi
    COPY ui/ .
    RUN npm run build
    
    # --- Go build ---
    FROM golang:1.22-alpine AS api
    WORKDIR /app
    COPY src/api/go.mod src/api/go.sum ./
    RUN go mod download
    COPY api/ .
    RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/ddui
    
    # --- Runtime ---
    FROM gcr.io/distroless/static-debian12
    WORKDIR /home/ddui
    COPY --from=api /bin/ddui /usr/local/bin/ddui
    COPY --from=ui /ui/dist /opt/ddui/ui
    ENV UI_DIST=/opt/ddui/ui
    ENV BIND_ADDR=:8080
    EXPOSE 8080
    USER 65532:65532
    ENTRYPOINT ["/usr/local/bin/ddui"]