# --- UI build ---
FROM node:20-alpine AS ui
WORKDIR /ui
COPY ui/package.json ui/package-lock.json* ./
RUN if [ -f package-lock.json ]; then \
        npm ci --include=dev --no-audit --no-fund; \
    else \
        npm install --include=dev --no-audit --no-fund; \
    fi
COPY ui/ .
RUN npm run build

# --- Rust build ---
FROM rust:1.85-slim AS builder
WORKDIR /app
COPY Cargo.toml ./
COPY src ./src
RUN cargo build --release
    
# --- Runtime ---
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /home/ddui
COPY --from=builder /app/target/release/ddui /usr/local/bin/ddui
COPY --from=ui /ui/dist ./ui/dist
ENV UI_DIST=/home/ddui/ui/dist
ENV BIND_ADDR=0.0.0.0:4421
EXPOSE 4421
ENV RUST_LOG=info

# Inline healthcheck (allowing extra startup time for DH param/gen etc.)
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=120s \
  CMD sh -c 'curl -fsS http://127.0.0.1:8080/healthz >/dev/null || exit 1'

CMD ["ddui"]