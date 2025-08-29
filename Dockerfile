# --- UI build ---
    FROM node:20-alpine AS ui
    WORKDIR /ui
    COPY ui/package*.json ./
    RUN npm ci
    COPY ui ./
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
    EXPOSE 8080
    ENV RUST_LOG=info
    CMD ["ddui"]