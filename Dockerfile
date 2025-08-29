FROM rust:1.85-slim AS builder
WORKDIR /app
COPY Cargo.toml ./
COPY src ./src
RUN cargo build --release

FROM debian:bookworm-slim
RUN useradd -u 10001 -m ddui && apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /home/ddui
COPY --from=builder /app/target/release/ddui /usr/local/bin/ddui

USER 10001
EXPOSE 3000
ENV RUST_LOG=info
CMD ["/usr/local/bin/ddui"]