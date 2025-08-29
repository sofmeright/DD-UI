# DDUI (Designated Driver UI) — Starter

Rust-based orchestrator skeleton with:
- `GET /api/healthz`
- `GET /api/inventory` (scans local `DDUI_SCAN_ROOT`)
- `POST /api/ci/run` (streams NDJSON stub)

## Quickstart

```bash
cargo run
# or with env:
DDUI_BIND=0.0.0.0:3000 DDUI_SCAN_KIND=local DDUI_SCAN_ROOT=./example/docker-compose cargo run
```

## Docker

```bash
docker build -t ddui:dev .
docker run --rm -p 3000:3000 -e DDUI_SCAN_ROOT=/data/docker-compose -v $(pwd)/example:/data ddui:dev
```

## Env

- `DDUI_BIND` (default `0.0.0.0:3000`)
- `DDUI_SCAN_KIND` `local|repo` (default `local`)
- `DDUI_SCAN_ROOT` (default `/opt/docker/ant-parade/docker-compose`)
- `DDUI_REFRESH_INTERVAL` (unused stub)
- `DDUI_LICENSE` or `DDUI_LICENSE_PATH` (optional; defaults to Community)

## License

Core intended for permissive license (Apache-2.0). See LICENSE.
